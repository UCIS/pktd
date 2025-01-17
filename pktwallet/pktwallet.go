// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/pktconfig/version"
	"github.com/pkt-cash/pktd/pktlog/log"

	"github.com/arl/statsviz"
	"github.com/pkt-cash/pktd/neutrino"
	"github.com/pkt-cash/pktd/pktwallet/chain"
	"github.com/pkt-cash/pktd/pktwallet/rpc/legacyrpc"
	"github.com/pkt-cash/pktd/pktwallet/wallet"
	"github.com/pkt-cash/pktd/pktwallet/walletdb"
)

var (
	cfg *config
)

func main() {
	version.SetUserAgentName("pktwallet")

	// After EXTENSIVE RUNTIME testing on multiple
	// platforms, this should be MIN 4 and maybe
	// closer to 8 - so 6 is it!
	// (esp to avoid GC lag in goleveldb - which
	//  still leaks memory on occasion that causes
	//  longer and longer GC runs.  ugh go.)
	runtime.GOMAXPROCS(runtime.NumCPU() * 6)

	// Work around defer not working after os.Exit.
	if err := walletMain(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.String())
		os.Exit(1)
	}
}

// walletMain is a work-around main function that is required since deferred
// functions (such as log flushing) are not called with calls to os.Exit.
// Instead, main runs this function and checks for a non-nil error, at which
// point any defers have already run, and if the error is non-nil, the program
// can be exited with an error exit status.
func walletMain() er.R {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	tcfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	cfg = tcfg

	// Show version at startup.
	log.Infof("Version %s", version.Version())
	log.WarnIfPrerelease()

	// Enable Profile server if requested.
	if cfg.Profile != "" {
		go func() {
			listenAddr := net.JoinHostPort("", cfg.Profile)
			log.Infof("Profile server listening on %s", listenAddr)
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			log.Errorf("%v", http.ListenAndServe(listenAddr, nil))
		}()
	}

	// Enable StatsViz server if requested.
	if cfg.StatsViz != "" {
		statsvizAddr := net.JoinHostPort("", cfg.StatsViz)
		log.Infof("StatsViz server listening on %s", statsvizAddr)
		svmux := http.NewServeMux()
		statsvizRedirect := http.RedirectHandler("/debug/statsviz", http.StatusSeeOther)
		svmux.Handle("/", statsvizRedirect)
		if err := statsviz.Register(svmux, statsviz.Root("/debug/statsviz")); err != nil {
			log.Errorf("%v", err)
		}
		go func() {
			log.Errorf("%v", http.ListenAndServe(statsvizAddr, svmux))
		}()
	}

	dbDir := networkDir(cfg.AppDataDir.Value, activeNet.Params)
	// TODO(cjd): noFreelistSync ?
	loader := wallet.NewLoader(activeNet.Params, dbDir, cfg.Wallet, false, 250)

	// Create and start HTTP server to serve wallet client connections.
	// This will be updated with the wallet and chain server RPC client
	// created below after each is created.
	rpcs, legacyRPCServer, err := startRPCServers(loader)
	if err != nil {
		log.Errorf("Unable to create RPC servers: %v", err)
		return err
	}

	// Create and start chain RPC client so it's ready to connect to
	// the wallet when loaded later.
	if !cfg.NoInitialLoad {
		go rpcClientConnectLoop(legacyRPCServer, loader)
	}

	loader.RunAfterLoad(func(w *wallet.Wallet) {
		startWalletRPCServices(w, rpcs, legacyRPCServer)
	})

	if !cfg.NoInitialLoad {
		// Load the wallet database.  It must have been created already
		// or this will return an appropriate error.
		_, err = loader.OpenExistingWallet([]byte(cfg.WalletPass), true)
		if err != nil {
			log.Error(err)
			return err
		}
	}

	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}
	}

	log.Info("Shutdown complete")
	return nil
}

// rpcClientConnectLoop continuously attempts a connection to the consensus RPC
// server.  When a connection is established, the client is used to sync the
// loaded wallet, either immediately or when loaded at a later time.
//
// The legacy RPC is optional.  If set, the connected RPC client will be
// associated with the server for RPC passthrough and to enable additional
// methods.
func rpcClientConnectLoop(legacyRPCServer *legacyrpc.Server, loader *wallet.Loader) {
	var certs []byte
	if cfg.UseRPC {
		certs = readCAFile()
	}

	for {
		var (
			chainClient chain.Interface
			err         er.R
		)

		if !cfg.UseRPC {
			var (
				chainService *neutrino.ChainService
				spvdb        walletdb.DB
			)
			netDir := networkDir(cfg.AppDataDir.Value, activeNet.Params)
			spvdb, err = walletdb.Create("bdb",
				filepath.Join(netDir, "neutrino.db"), false)
			defer spvdb.Close()
			if err != nil {
				log.Errorf("Unable to create Neutrino DB: %s", err)
				continue
			}
			cp := cfg.ConnectPeers
			chainService, err = neutrino.NewChainService(
				neutrino.Config{
					DataDir:      netDir,
					Database:     spvdb,
					ChainParams:  *activeNet.Params,
					ConnectPeers: cp,
					AddPeers:     cfg.AddPeers,
				})
			if err != nil {
				log.Errorf("Couldn't create Neutrino ChainService: %s", err)
				continue
			}
			chainClient = chain.NewNeutrinoClient(activeNet.Params, chainService)
			err = chainClient.Start()
			if err != nil {
				log.Errorf("Couldn't start Neutrino client: %s", err)
			}
		} else {
			chainClient, err = startChainRPC(certs)
			if err != nil {
				log.Errorf("Unable to open connection to consensus RPC server: %v", err)
				continue
			}
		}

		// Rather than inlining this logic directly into the loader
		// callback, a function variable is used to avoid running any of
		// this after the client disconnects by setting it to nil.  This
		// prevents the callback from associating a wallet loaded at a
		// later time with a client that has already disconnected.  A
		// mutex is used to make this concurrent safe.
		associateRPCClient := func(w *wallet.Wallet) {
			w.SynchronizeRPC(chainClient)
			if legacyRPCServer != nil {
				legacyRPCServer.SetChainServer(chainClient)
			}
		}
		mu := new(sync.Mutex)
		loader.RunAfterLoad(func(w *wallet.Wallet) {
			mu.Lock()
			associate := associateRPCClient
			mu.Unlock()
			if associate != nil {
				associate(w)
			}
		})

		chainClient.WaitForShutdown()

		mu.Lock()
		associateRPCClient = nil
		mu.Unlock()

		loadedWallet, ok := loader.LoadedWallet()
		if ok {
			// Do not attempt a reconnect when the wallet was
			// explicitly stopped.
			if loadedWallet.ShuttingDown() {
				return
			}

			loadedWallet.SetChainSynced(false)

			// TODO: Rework the wallet so changing the RPC client
			// does not require stopping and restarting everything.
			loadedWallet.Stop()
			loadedWallet.WaitForShutdown()
			loadedWallet.Start()
		}
	}
}

func readCAFile() []byte {
	// Read certificate file if TLS is not disabled.
	var certs []byte
	if cfg.ClientTLS {
		var errr error
		certs, errr = ioutil.ReadFile(cfg.CAFile.Value)
		if errr != nil {
			log.Warnf("Cannot open CA file: %v", errr)
			// If there's an error reading the CA file, continue
			// with nil certs and without the client connection.
			certs = nil
		}
	}

	return certs
}

// startChainRPC opens a RPC client connection to a pktd server for blockchain
// services.  This function uses the RPC options from the global config and
// there is no recovery in case the server is not available or if there is an
// authentication error.  Instead, all requests to the client will simply error.
func startChainRPC(certs []byte) (*chain.RPCClient, er.R) {
	log.Infof("Attempting RPC client connection to %v", cfg.RPCConnect)
	rpcc, err := chain.NewRPCClient(activeNet.Params, cfg.RPCConnect,
		cfg.BtcdUsername, cfg.BtcdPassword, certs, !cfg.ClientTLS, 0)
	if err != nil {
		return nil, err
	}
	err = rpcc.Start()
	return rpcc, err
}
