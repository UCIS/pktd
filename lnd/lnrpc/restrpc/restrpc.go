package restrpc

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/mux"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkt-cash/pktd/btcjson"
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/connmgr/banmgr"
	"github.com/pkt-cash/pktd/lnd/chainreg"
	"github.com/pkt-cash/pktd/lnd/lnrpc"
	"github.com/pkt-cash/pktd/lnd/lnrpc/routerrpc"
	"github.com/pkt-cash/pktd/lnd/lnrpc/verrpc"
	"github.com/pkt-cash/pktd/lnd/walletunlocker"
	"github.com/pkt-cash/pktd/neutrino"
	"github.com/pkt-cash/pktd/pktlog/log"
	"github.com/pkt-cash/pktd/pktwallet/waddrmgr"
	"github.com/pkt-cash/pktd/pktwallet/wallet"
	"github.com/pkt-cash/pktd/pktwallet/wallet/seedwords"
)

type RpcFunc struct {
	path string
	req  proto.Message
	res  proto.Message
	f    func(c *RpcContext, m proto.Message) (proto.Message, er.R)
}

var hexSeedRegex = regexp.MustCompile(`^[0-9a-f]{64}$`)

var rpcFunctions []RpcFunc = []RpcFunc{
	//WalletUnlocker: Wallet unlock
	//Will try to unlock the wallet with the password(s) provided
	{
		path: "/api/v1/wallet/unlock",
		req:  (*lnrpc.UnlockWalletRequest)(nil),
		res:  nil,
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.UnlockWalletRequest)
			if !ok {
				return nil, er.New("Argument is not a UnlockWalletRequest")
			}
			if u, err := c.withUnlocker(); err != nil {
				return nil, err
			} else if _, err := u.UnlockWallet0(context.TODO(), req); err != nil {
				return nil, err
			}
			return nil, nil
		},
	},
	//WalletUnlocker: Wallet create
	//Will try to create/restore wallet
	{
		path: "/api/v1/wallet/create",
		req:  (*lnrpc.CreateWalletRequest)(nil),
		res:  (*lnrpc.CreateWalletResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.CreateWalletRequest)
			if !ok {
				return nil, er.New("Argument is not a CreateWalletRequest")
			}
			u, err := c.withUnlocker()
			if err != nil {
				return nil, err
			}
			resp, errr := u.CreateWallet(context.TODO(), req)
			if errr != nil {
				return nil, er.E(errr)
			}
			return resp, nil
		},
	},
	{
		path: "/api/v1/util/checkseed",
		req:  (*lnrpc.CheckSeedRequest)(nil),
		res:  (*lnrpc.CheckSeedResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.CheckSeedRequest)
			if !ok {
				return nil, er.New("Argument is not a CheckSeedRequest")
			}
			if len(req.SeedWords) == 1 && hexSeedRegex.MatchString(req.SeedWords[0]) {
				return &lnrpc.CheckSeedResponse{
					SeedType: lnrpc.SeedType_ST_LEGACY,
				}, nil
			}
			seedEnc, err := seedwords.SeedFromWords(strings.Join(req.SeedWords, " "))
			if err != nil {
				return nil, err
			}
			if seedEnc.NeedsPassphrase() {
				return &lnrpc.CheckSeedResponse{
					SeedType: lnrpc.SeedType_ST_ENCRYPTED,
				}, nil
			} else {
				return &lnrpc.CheckSeedResponse{
					SeedType: lnrpc.SeedType_ST_UNENCRYPTED,
				}, nil
			}
		},
	},
	//MetaService get info
	{
		path: "/api/v1/meta/getinfo",
		req:  nil,
		res:  (*lnrpc.GetInfo2Response)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			var ni lnrpc.NeutrinoInfo
			if n, _ := c.withNeutrino(); n != nil {
				neutrinoPeers := n.Peers()
				for i := range neutrinoPeers {
					var peerDesc lnrpc.PeerDesc
					neutrinoPeer := neutrinoPeers[i]

					peerDesc.BytesReceived = neutrinoPeer.BytesReceived()
					peerDesc.BytesSent = neutrinoPeer.BytesSent()
					peerDesc.LastRecv = neutrinoPeer.LastRecv().String()
					peerDesc.LastSend = neutrinoPeer.LastSend().String()
					peerDesc.Connected = neutrinoPeer.Connected()
					peerDesc.Addr = neutrinoPeer.Addr()
					peerDesc.Inbound = neutrinoPeer.Inbound()
					na := neutrinoPeer.NA()
					if na != nil {
						peerDesc.Na = na.IP.String() + ":" + strconv.Itoa(int(na.Port))
					}
					peerDesc.Id = neutrinoPeer.ID()
					peerDesc.UserAgent = neutrinoPeer.UserAgent()
					peerDesc.Services = neutrinoPeer.Services().String()
					peerDesc.VersionKnown = neutrinoPeer.VersionKnown()
					peerDesc.AdvertisedProtoVer = neutrinoPeer.Describe().AdvertisedProtoVer
					peerDesc.ProtocolVersion = neutrinoPeer.ProtocolVersion()
					peerDesc.SendHeadersPreferred = neutrinoPeer.Describe().SendHeadersPreferred
					peerDesc.VerAckReceived = neutrinoPeer.VerAckReceived()
					peerDesc.WitnessEnabled = neutrinoPeer.Describe().WitnessEnabled
					peerDesc.WireEncoding = strconv.Itoa(int(neutrinoPeer.Describe().WireEncoding))
					peerDesc.TimeOffset = neutrinoPeer.TimeOffset()
					peerDesc.TimeConnected = neutrinoPeer.Describe().TimeConnected.String()
					peerDesc.StartingHeight = neutrinoPeer.StartingHeight()
					peerDesc.LastBlock = neutrinoPeer.LastBlock()
					if neutrinoPeer.LastAnnouncedBlock() != nil {
						peerDesc.LastAnnouncedBlock = neutrinoPeer.LastAnnouncedBlock().CloneBytes()
					}
					peerDesc.LastPingNonce = neutrinoPeer.LastPingNonce()
					peerDesc.LastPingTime = neutrinoPeer.LastPingTime().String()
					peerDesc.LastPingMicros = neutrinoPeer.LastPingMicros()

					ni.Peers = append(ni.Peers, &peerDesc)
				}
				n.BanMgr().ForEachIp(func(bi banmgr.BanInfo) er.R {
					ban := lnrpc.NeutrinoBan{}
					ban.Addr = bi.Addr
					ban.Reason = bi.Reason
					ban.EndTime = bi.BanExpiresTime.String()
					ban.BanScore = bi.BanScore

					ni.Bans = append(ni.Bans, &ban)
					return nil
				})

				neutrionoQueries := n.GetActiveQueries()
				for i := range neutrionoQueries {
					nq := lnrpc.NeutrinoQuery{}
					query := neutrionoQueries[i]
					if query.Peer != nil {
						nq.Peer = query.Peer.String()
					} else {
						nq.Peer = "<nil>"
					}
					nq.Command = query.Command
					nq.ReqNum = query.ReqNum
					nq.CreateTime = query.CreateTime
					nq.LastRequestTime = query.LastRequestTime
					nq.LastResponseTime = query.LastResponseTime

					ni.Queries = append(ni.Queries, &nq)
				}

				bb, err := n.BestBlock()
				if err != nil {
					return nil, err
				}
				ni.BlockHash = bb.Hash.String()
				ni.Height = bb.Height
				ni.BlockTimestamp = bb.Timestamp.String()
				ni.IsSyncing = !n.IsCurrent()
			}

			var walletInfo *lnrpc.WalletInfo
			if w, _ := c.withWallet(); w != nil {
				mgrStamp := w.Manager.SyncedTo()
				walletStats := &lnrpc.WalletStats{}
				w.ReadStats(func(ws *btcjson.WalletStats) {
					walletStats.MaintenanceInProgress = ws.MaintenanceInProgress
					walletStats.MaintenanceName = ws.MaintenanceName
					walletStats.MaintenanceCycles = int32(ws.MaintenanceCycles)
					walletStats.MaintenanceLastBlockVisited = int32(ws.MaintenanceLastBlockVisited)
					walletStats.Syncing = ws.Syncing
					if ws.SyncStarted != nil {
						walletStats.SyncStarted = ws.SyncStarted.String()
					}
					walletStats.SyncRemainingSeconds = ws.SyncRemainingSeconds
					walletStats.SyncCurrentBlock = ws.SyncCurrentBlock
					walletStats.SyncFrom = ws.SyncFrom
					walletStats.SyncTo = ws.SyncTo
					walletStats.BirthdayBlock = ws.BirthdayBlock
				})
				walletInfo = &lnrpc.WalletInfo{
					CurrentBlockHash:      mgrStamp.Hash.String(),
					CurrentHeight:         mgrStamp.Height,
					CurrentBlockTimestamp: mgrStamp.Timestamp.String(),
					WalletVersion:         int32(waddrmgr.LatestMgrVersion),
					WalletStats:           walletStats,
				}
			}

			// Get Lightning info
			var lightning *lnrpc.GetInfoResponse
			if cc, _ := c.withRpcServer(); cc != nil {
				if l, err := cc.GetInfo(context.TODO(), nil); err != nil {
					return nil, er.E(err)
				} else {
					lightning = l
				}
			}

			return &lnrpc.GetInfo2Response{
				Neutrino:  &ni,
				Wallet:    walletInfo,
				Lightning: lightning,
			}, nil
		},
	},
	//MetaService change wallet password
	{
		path: "/api/v1/meta/changepassword",
		req:  (*lnrpc.ChangePasswordRequest)(nil),
		res:  (*lnrpc.ChangePasswordResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.ChangePasswordRequest)
			if !ok {
				return nil, er.New("Argument is not a ChangePasswordRequest")
			}
			meta, err := c.withMetaServer()
			if err != nil {
				return nil, err
			}
			resp, errr := meta.ChangePassword(context.TODO(), req)
			if errr != nil {
				return nil, er.E(errr)
			}
			return resp, nil
		},
	},
	//Wallet balance
	//requires unlocked wallet -> access to rpcServer
	{
		path: "/api/v1/lightning/walletbalance",
		req:  nil,
		res:  (*lnrpc.GetAddressBalancesResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.WalletBalance(context.TODO(), nil); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//Wallet transactions
	//requires unlocked wallet -> access to rpcServer
	{
		path: "/api/v1/lightning/gettransactions",
		req:  (*lnrpc.GetTransactionsRequest)(nil),
		res:  (*lnrpc.TransactionDetails)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.GetTransactionsRequest)
			if !ok {
				return nil, er.New("Argument is not a GetTransactionsRequest")
			}
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.GetTransactions(context.TODO(), req); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//New wallet address
	//requires unlocked wallet -> access to rpcServer
	{
		path: "/api/v1/lightning/getnewaddress",
		req:  (*lnrpc.GetNewAddressRequest)(nil),
		res:  (*lnrpc.GetNewAddressResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.GetNewAddressRequest)
			if !ok {
				return nil, er.New("Argument is not a GetAddressBalancesRequest")
			}
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.GetNewAddress(context.TODO(), req); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//GetAddressBalances
	{
		path: "/api/v1/lightning/getaddressbalances",
		req:  (*lnrpc.GetAddressBalancesRequest)(nil),
		res:  (*lnrpc.GetAddressBalancesResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.GetAddressBalancesRequest)
			if !ok {
				return nil, er.New("Argument is not a GetAddressBalancesRequest")
			}
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.GetAddressBalances(context.TODO(), req); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//Sendfrom
	{
		path: "/api/v1/lightning/sendfrom",
		req:  (*lnrpc.SendFromRequest)(nil),
		res:  (*lnrpc.SendFromResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.SendFromRequest)
			if !ok {
				return nil, er.New("Argument is not a SendFromRequest")
			}
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.SendFrom(context.TODO(), req); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//GetWalletSeed
	{
		path: "/api/v1/lightning/getwalletseed",
		req:  nil,
		res:  (*lnrpc.GetWalletSeedResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.GetWalletSeed(context.TODO(), nil); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//GetTransaction
	{
		path: "/api/v1/lightning/gettransaction",
		req:  (*lnrpc.GetTransactionRequest)(nil),
		res:  (*lnrpc.GetTransactionResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.GetTransactionRequest)
			if !ok {
				return nil, er.New("Argument is not a GetTransactionRequest")
			}
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.GetTransaction(context.TODO(), req); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//Resync
	{
		path: "/api/v1/lightning/resync",
		req:  (*lnrpc.ReSyncChainRequest)(nil),
		res:  (*lnrpc.ReSyncChainResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			req, ok := m.(*lnrpc.ReSyncChainRequest)
			if !ok {
				return nil, er.New("Argument is not a ReSyncChainRequest")
			}
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.ReSync(context.TODO(), req); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//StopResync
	{
		path: "/api/v1/lightning/stopresync",
		req:  nil,
		res:  (*lnrpc.StopReSyncResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {
			if server, err := c.withRpcServer(); server != nil {
				if l, err := server.StopReSync(context.TODO(), nil); err != nil {
					return nil, er.E(err)
				} else {
					return l, nil
				}
			} else {
				return nil, err
			}
		},
	},
	//	GenSeed service
	{
		path: "/api/v1/lightning/genseed",
		req:  (*lnrpc.GenSeedRequest)(nil),
		res:  (*lnrpc.GenSeedResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			genSeedReq, ok := m.(*lnrpc.GenSeedRequest)
			if !ok {
				return nil, er.New("Argument is not a GenSeedRequest")
			}

			//	generate a new seed
			cc, errr := c.withUnlocker()
			if cc != nil {
				var genSeedResp *lnrpc.GenSeedResponse

				genSeedResp, err := cc.GenSeed(context.TODO(), genSeedReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return genSeedResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	meta service get recovery info
	{
		path: "/api/v1/meta/getrecoveryinfo",
		req:  nil,
		res:  (*lnrpc.GetRecoveryInfoResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get Lightning recovery info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var recoveryInfoResp *lnrpc.GetRecoveryInfoResponse

				recoveryInfoResp, err := cc.GetRecoveryInfo(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return recoveryInfoResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service debug level
	{
		path: "/api/v1/meta/debuglevel",
		req:  (*lnrpc.DebugLevelRequest)(nil),
		res:  (*lnrpc.DebugLevelResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			debugLevelReq, ok := m.(*lnrpc.DebugLevelRequest)
			if !ok {
				return nil, er.New("Argument is not a DebugLevelRequest")
			}

			//	set Lightning debug level
			cc, errr := c.withRpcServer()
			if cc != nil {
				var debugLevelResp *lnrpc.DebugLevelResponse

				debugLevelResp, err := cc.DebugLevel(context.TODO(), debugLevelReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return debugLevelResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service to stop the pld daemon
	{
		path: "/api/v1/meta/stop",
		req:  nil,
		res:  (*lnrpc.StopResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	invoke Lightning stop daemon command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var stopResp *lnrpc.StopResponse

				stopResp, err := cc.StopDaemon(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return stopResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	TODO: service daemon version
	{
		path: "/api/v1/meta/version",
		req:  nil,
		res:  (*verrpc.Version)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get daemon version
			cc, errr := c.withVerRPCServer()
			if cc != nil {
				var versionResp *verrpc.Version

				versionResp, err := cc.GetVersion(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return versionResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	TODO: service openchannel
	/*
		{
			path: "/api/v1/channels/open",
			req:  (*lnrpc.OpenChannelRequest)(nil),
			res:  nil,
			f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

				//	get the request payload
				openChannelReq, ok := m.(*lnrpc.OpenChannelRequest)
				if !ok {
					return nil, er.New("Argument is not a OpenChannelRequest")
				}

				//	open a channel
				cc, errr := c.withRpcServer()
				if cc != nil {
					err := cc.OpenChannel(openChannelReq, lightningOpenChannelServer{stream})
					if err != nil {
						return nil, er.E(err)
					} else {
						return nil, nil
					}
				} else {
					return nil, errr
				}
			},
		},
	*/
	//	TODO: service closechannel
	//	check with Dimitris if the URL parameters should be validated with the payload, and fill the payload in the case they came only on URL
	{
		path: "/api//v1/channels/close",
		req:  (*lnrpc.CloseChannelRequest)(nil),
		res:  nil,
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			closeChannelReq, ok := m.(*lnrpc.CloseChannelRequest)
			if !ok {
				return nil, er.New("Argument is not a CloseChannelRequest")
			}

			//	close a channel
			cc, errr := c.withRpcServer()
			if cc != nil {
				err := cc.CloseChannel(closeChannelReq, nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return nil, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	TODO: service closeallchannels
	//	check with Dimitris because the CloseAllChannels calls listChannels and then close one by one. This means the Payload for this REST command needs to be created !
	/*
		{
			path: "/api/v1/channels/closeall",
			req:  nil,
			res:  (*lnrpc.GetRecoveryInfoResponse)(nil),
			f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

				//	close all channels
				cc, errr := c.withRpcServer()
				if cc != nil {
					var recoveryInfo *lnrpc.GetRecoveryInfoResponse

					recoveryInfo, err := cc.GetRecoveryInfo(context.TODO(), nil)
					if err != nil {
						return nil, er.E(err)
					} else {
						return recoveryInfo, nil
					}
				} else {
					return nil, errr
				}
			},
		},
	*/
	//	TODO: service abandonchannel
	//	check with Dimitris if the URL parameters should be validated with the payload, and fill the payload in the case they came only on URL
	{
		path: "/api/v1/channel/abandon",
		req:  (*lnrpc.AbandonChannelRequest)(nil),
		res:  (*lnrpc.AbandonChannelResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			abandonChannelReq, ok := m.(*lnrpc.AbandonChannelRequest)
			if !ok {
				return nil, er.New("Argument is not a AbandonChannelRequest")
			}

			//	abandon a channel
			cc, errr := c.withRpcServer()
			if cc != nil {
				var abandonChannelResp *lnrpc.AbandonChannelResponse

				abandonChannelResp, err := cc.AbandonChannel(context.TODO(), abandonChannelReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return abandonChannelResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service channelbalance
	{
		path: "/api/v1/channel/balance",
		req:  nil,
		res:  (*lnrpc.ChannelBalanceResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the channel balance
			cc, errr := c.withRpcServer()
			if cc != nil {
				var channelBalanceResp *lnrpc.ChannelBalanceResponse

				channelBalanceResp, err := cc.ChannelBalance(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return channelBalanceResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service pendingchannels
	{
		path: "/api/v1/channel/pending",
		req:  nil,
		res:  (*lnrpc.PendingChannelsResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get pending channels info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var pendingChannelsResp *lnrpc.PendingChannelsResponse

				pendingChannelsResp, err := cc.PendingChannels(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return pendingChannelsResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service listchannels
	{
		path: "/api/v1/channel",
		req:  (*lnrpc.ListChannelsRequest)(nil),
		res:  (*lnrpc.ListChannelsResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			listChannelsReq, ok := m.(*lnrpc.ListChannelsRequest)
			if !ok {
				return nil, er.New("Argument is not a ListChannelsRequest")
			}

			//	get a list of channels
			cc, errr := c.withRpcServer()
			if cc != nil {
				var listChannelsResp *lnrpc.ListChannelsResponse

				listChannelsResp, err := cc.ListChannels(context.TODO(), listChannelsReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return listChannelsResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service closedchannels
	{
		path: "/api/v1/channel/closed",
		req:  (*lnrpc.ClosedChannelsRequest)(nil),
		res:  (*lnrpc.ClosedChannelsResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			closeChannelReq, ok := m.(*lnrpc.ClosedChannelsRequest)
			if !ok {
				return nil, er.New("Argument is not a ClosedChannelRequest")
			}

			//	get a list of all closed channels
			cc, errr := c.withRpcServer()
			if cc != nil {
				var closedChannelsResp *lnrpc.ClosedChannelsResponse

				closedChannelsResp, err := cc.ClosedChannels(context.TODO(), closeChannelReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return closedChannelsResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service getnetworkinfo
	{
		path: "/api/v1/channel/networkinfo",
		req:  nil,
		res:  (*lnrpc.NetworkInfo)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get network info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var networkInfoResp *lnrpc.NetworkInfo

				networkInfoResp, err := cc.GetNetworkInfo(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return networkInfoResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service feereport
	{
		path: "/api/v1/channel/feereport",
		req:  nil,
		res:  (*lnrpc.FeeReportResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get fee report
			cc, errr := c.withRpcServer()
			if cc != nil {
				var feeReportResp *lnrpc.FeeReportResponse

				feeReportResp, err := cc.FeeReport(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return feeReportResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service updatechanpolicy
	{
		path: "/api/v1/channel/policy",
		req:  (*lnrpc.PolicyUpdateRequest)(nil),
		res:  (*lnrpc.PolicyUpdateResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			policyUpdateReq, ok := m.(*lnrpc.PolicyUpdateRequest)
			if !ok {
				return nil, er.New("Argument is not a ClosedChannelRequest")
			}

			//	invoke Lightning update chan policy command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var policyUpdateResp *lnrpc.PolicyUpdateResponse

				policyUpdateResp, err := cc.UpdateChannelPolicy(context.TODO(), policyUpdateReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return policyUpdateResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service exportchanbackup
	{
		path: "/api/v1/channel/backup/export",
		req:  (*lnrpc.ExportChannelBackupRequest)(nil),
		res:  (*lnrpc.ChannelBackup)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			exportChannelBackupReq, ok := m.(*lnrpc.ExportChannelBackupRequest)
			if !ok {
				return nil, er.New("Argument is not a ExportChannelBackupRequest")
			}

			//	invoke Lightning export chan backup command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var channelBackupResp *lnrpc.ChannelBackup

				channelBackupResp, err := cc.ExportChannelBackup(context.TODO(), exportChannelBackupReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return channelBackupResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service verifychanbackup
	{
		path: "/api/v1/channel/backup/verify",
		req:  (*lnrpc.ChanBackupSnapshot)(nil),
		res:  (*lnrpc.VerifyChanBackupResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			chanBackupSnapshotReq, ok := m.(*lnrpc.ChanBackupSnapshot)
			if !ok {
				return nil, er.New("Argument is not a ChanBackupSnapshot")
			}

			//	invoke Lightning verify chan backup command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var verifyChanBackupResp *lnrpc.VerifyChanBackupResponse

				verifyChanBackupResp, err := cc.VerifyChanBackup(context.TODO(), chanBackupSnapshotReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return verifyChanBackupResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service restorechanbackup
	{
		path: "/api/v1/channel/backup/restore",
		req:  (*lnrpc.RestoreChanBackupRequest)(nil),
		res:  (*lnrpc.RestoreBackupResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			restoreChanBackupReq, ok := m.(*lnrpc.RestoreChanBackupRequest)
			if !ok {
				return nil, er.New("Argument is not a RestoreChanBackupRequest")
			}

			//	invoke Lightning restore chan backup command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var restoreBackupResp *lnrpc.RestoreBackupResponse

				restoreBackupResp, err := cc.RestoreChannelBackups(context.TODO(), restoreChanBackupReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return restoreBackupResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service describegraph
	{
		path: "/api/v1/graph",
		req:  (*lnrpc.ChannelGraphRequest)(nil),
		res:  (*lnrpc.ChannelGraph)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			channelGraphReq, ok := m.(*lnrpc.ChannelGraphRequest)
			if !ok {
				return nil, er.New("Argument is not a ChannelGraphRequest")
			}

			//	get graph description info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var channelGraphResp *lnrpc.ChannelGraph

				channelGraphResp, err := cc.DescribeGraph(context.TODO(), channelGraphReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return channelGraphResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service getnodemetrics
	{
		path: "/api/v1/graph/nodemetrics",
		req:  (*lnrpc.NodeMetricsRequest)(nil),
		res:  (*lnrpc.NodeMetricsResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			nodeMetricsReq, ok := m.(*lnrpc.NodeMetricsRequest)
			if !ok {
				return nil, er.New("Argument is not a NodeMetricsRequest")
			}

			//	get node metrics info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var nodeMetricsResp *lnrpc.NodeMetricsResponse

				nodeMetricsResp, err := cc.GetNodeMetrics(context.TODO(), nodeMetricsReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return nodeMetricsResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service getchaninfo
	{
		path: "/api/v1/graph/channel",
		req:  (*lnrpc.ChanInfoRequest)(nil),
		res:  (*lnrpc.ChannelEdge)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			chanInfoReq, ok := m.(*lnrpc.ChanInfoRequest)
			if !ok {
				return nil, er.New("Argument is not a ChanInfoRequest")
			}

			//	get chan info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var channelEdgeResp *lnrpc.ChannelEdge

				channelEdgeResp, err := cc.GetChanInfo(context.TODO(), chanInfoReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return channelEdgeResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service getnodeinfo
	{
		path: "/api/v1/graph/nodeinfo",
		req:  (*lnrpc.NodeInfoRequest)(nil),
		res:  (*lnrpc.NodeInfo)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			nodeInfoReq, ok := m.(*lnrpc.NodeInfoRequest)
			if !ok {
				return nil, er.New("Argument is not a NodeInfoRequest")
			}

			//	get node info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var nodeInfoResp *lnrpc.NodeInfo

				nodeInfoResp, err := cc.GetNodeInfo(context.TODO(), nodeInfoReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return nodeInfoResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service addinvoice
	{
		path: "/api/v1/invoice/add",
		req:  (*lnrpc.Invoice)(nil),
		res:  (*lnrpc.AddInvoiceResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			invoiceReq, ok := m.(*lnrpc.Invoice)
			if !ok {
				return nil, er.New("Argument is not a Invoice")
			}

			//	add an invoice
			cc, errr := c.withRpcServer()
			if cc != nil {
				var addInvoiceResp *lnrpc.AddInvoiceResponse

				addInvoiceResp, err := cc.AddInvoice(context.TODO(), invoiceReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return addInvoiceResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service lookupinvoice
	{
		path: "/api/v1/invoice/lookup",
		req:  (*lnrpc.PaymentHash)(nil),
		res:  (*lnrpc.Invoice)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			paymentHashReq, ok := m.(*lnrpc.PaymentHash)
			if !ok {
				return nil, er.New("Argument is not a PaymentHash")
			}

			//	lookup an invoice
			cc, errr := c.withRpcServer()
			if cc != nil {
				var InvoiceResp *lnrpc.Invoice

				InvoiceResp, err := cc.LookupInvoice(context.TODO(), paymentHashReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return InvoiceResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service listinvoices
	{
		path: "/api/v1/invoice",
		req:  (*lnrpc.ListInvoiceRequest)(nil),
		res:  (*lnrpc.ListInvoiceResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			listInvoiceReq, ok := m.(*lnrpc.ListInvoiceRequest)
			if !ok {
				return nil, er.New("Argument is not a ListInvoiceRequest")
			}

			//	list all invoices
			cc, errr := c.withRpcServer()
			if cc != nil {
				var listInvoiceResp *lnrpc.ListInvoiceResponse

				listInvoiceResp, err := cc.ListInvoices(context.TODO(), listInvoiceReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return listInvoiceResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service decodepayreq
	{
		path: "/api/v1/invoice/decodepayreq",
		req:  (*lnrpc.PayReqString)(nil),
		res:  (*lnrpc.PayReq)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			payReqStringReq, ok := m.(*lnrpc.PayReqString)
			if !ok {
				return nil, er.New("Argument is not a PayReqString")
			}

			//	decode payment request
			cc, errr := c.withRpcServer()
			if cc != nil {
				var payReqResp *lnrpc.PayReq

				payReqResp, err := cc.DecodePayReq(context.TODO(), payReqStringReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return payReqResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service estimatefee
	{
		path: "/api/v1/transaction/estimatefee",
		req:  (*lnrpc.EstimateFeeRequest)(nil),
		res:  (*lnrpc.EstimateFeeResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			estimateFeeReq, ok := m.(*lnrpc.EstimateFeeRequest)
			if !ok {
				return nil, er.New("Argument is not a EstimateFeeRequest")
			}

			//	get estimate fee info
			cc, errr := c.withRpcServer()
			if cc != nil {
				var estimateFeeResp *lnrpc.EstimateFeeResponse

				estimateFeeResp, err := cc.EstimateFee(context.TODO(), estimateFeeReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return estimateFeeResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service sendmany
	{
		path: "/api/v1/transaction/sendmany",
		req:  (*lnrpc.SendManyRequest)(nil),
		res:  (*lnrpc.SendManyResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			sendManyReq, ok := m.(*lnrpc.SendManyRequest)
			if !ok {
				return nil, er.New("Argument is not a SendManyRequest")
			}

			//	send coins to many addresses
			cc, errr := c.withRpcServer()
			if cc != nil {
				var sendManyResp *lnrpc.SendManyResponse

				sendManyResp, err := cc.SendMany(context.TODO(), sendManyReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return sendManyResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service sendcoins
	{
		path: "/api/v1/transaction/sendcoins",
		req:  (*lnrpc.SendCoinsRequest)(nil),
		res:  (*lnrpc.SendCoinsResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			sendCoinsReq, ok := m.(*lnrpc.SendCoinsRequest)
			if !ok {
				return nil, er.New("Argument is not a SendCoinsRequest")
			}

			//	send coins to one addresses
			cc, errr := c.withRpcServer()
			if cc != nil {
				var sendCoinsResp *lnrpc.SendCoinsResponse

				sendCoinsResp, err := cc.SendCoins(context.TODO(), sendCoinsReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return sendCoinsResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service listunspent
	{
		path: "/api/v1/transaction/listunspent",
		req:  (*lnrpc.ListUnspentRequest)(nil),
		res:  (*lnrpc.ListUnspentResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			listUnspentReq, ok := m.(*lnrpc.ListUnspentRequest)
			if !ok {
				return nil, er.New("Argument is not a ListUnspentRequest")
			}

			//	get a list of available utxos
			cc, errr := c.withRpcServer()
			if cc != nil {
				var listUnspentResp *lnrpc.ListUnspentResponse

				listUnspentResp, err := cc.ListUnspent(context.TODO(), listUnspentReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return listUnspentResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service listchaintrns
	{
		path: "/api/v1/transaction",
		req:  (*lnrpc.GetTransactionsRequest)(nil),
		res:  (*lnrpc.TransactionDetails)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			getTransactionsReq, ok := m.(*lnrpc.GetTransactionsRequest)
			if !ok {
				return nil, er.New("Argument is not a GetTransactionsRequest")
			}

			//	get a list of transactions from wallet
			cc, errr := c.withRpcServer()
			if cc != nil {
				var transactionDetailsResp *lnrpc.TransactionDetails

				transactionDetailsResp, err := cc.GetTransactions(context.TODO(), getTransactionsReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return transactionDetailsResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service setnetworkstewardvote
	{
		path: "/api/v1/transaction/setnetworkstewardvote",
		req:  (*lnrpc.SetNetworkStewardVoteRequest)(nil),
		res:  (*lnrpc.SetNetworkStewardVoteResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			setNetworkStewardVoteReq, ok := m.(*lnrpc.SetNetworkStewardVoteRequest)
			if !ok {
				return nil, er.New("Argument is not a SetNetworkStewardVoteRequest")
			}

			//	set network steward vote
			cc, errr := c.withRpcServer()
			if cc != nil {
				var setNetworkStewardVoteResp *lnrpc.SetNetworkStewardVoteResponse

				setNetworkStewardVoteResp, err := cc.SetNetworkStewardVote(context.TODO(), setNetworkStewardVoteReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return setNetworkStewardVoteResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service getnetworkstewardvote
	{
		path: "/api/v1/transaction/getnetworkstewardvote",
		req:  nil,
		res:  (*lnrpc.GetNetworkStewardVoteResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get network steward vote
			cc, errr := c.withRpcServer()
			if cc != nil {
				var getNetworkStewardVoteResp *lnrpc.GetNetworkStewardVoteResponse

				getNetworkStewardVoteResp, err := cc.GetNetworkStewardVote(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return getNetworkStewardVoteResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service bcasttransaction
	{
		path: "/api/v1/transaction/bcast",
		req:  (*lnrpc.BcastTransactionRequest)(nil),
		res:  (*lnrpc.BcastTransactionResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			bcastTransactionReq, ok := m.(*lnrpc.BcastTransactionRequest)
			if !ok {
				return nil, er.New("Argument is not a BcastTransactionRequest")
			}

			//	invoke Lightning broadcast transaction in chain command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var bcastTransactionResp *lnrpc.BcastTransactionResponse

				bcastTransactionResp, err := cc.BcastTransaction(context.TODO(), bcastTransactionReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return bcastTransactionResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service sendpayment
	{
		path: "/api/v1/payment/send",
		req:  (*lnrpc.SendRequest)(nil),
		res:  (*lnrpc.SendResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			sendReq, ok := m.(*lnrpc.SendRequest)
			if !ok {
				return nil, er.New("Argument is not a SendRequest")
			}

			//	invoke Lightning send payment command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var sendResp *lnrpc.SendResponse

				sendResp, err := cc.SendPaymentSync(context.TODO(), sendReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return sendResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	TODO: service payinvoice
	/*
		check payInvoice incm_pay.go source file
		{
			path: "/api/v1/payment/payinvoice",
			req:  (*routerrpc.SendPaymentRequest)(nil),
			res:  nil,
			f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

				//	get the request payload
				sendPaymentReq, ok := m.(*routerrpc.SendPaymentRequest)
				if !ok {
					return nil, er.New("Argument is not a SendPaymentRequest")
				}

				//	query the mission control status
				cc, errr := c.withRouterServer()
				if cc != nil {
					err := cc.SendPaymentV2(sendPaymentReq, )
					if err != nil {
						return nil, er.E(err)
					} else {
						return nil, nil
					}
				} else {
					return nil, errr
				}
			},
		},
	*/
	//	service sendtoroute
	{
		path: "/api/v1/payment/sendroute",
		req:  (*lnrpc.SendToRouteRequest)(nil),
		res:  (*lnrpc.SendResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			sendToRouteReq, ok := m.(*lnrpc.SendToRouteRequest)
			if !ok {
				return nil, er.New("Argument is not a SendToRouteRequest")
			}

			//	invoke Lightning send to route command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var sendResp *lnrpc.SendResponse

				sendResp, err := cc.SendToRouteSync(context.TODO(), sendToRouteReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return sendResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service listpayments
	{
		path: "/api/v1/payment",
		req:  (*lnrpc.ListPaymentsRequest)(nil),
		res:  (*lnrpc.ListPaymentsResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			listPaymentsReq, ok := m.(*lnrpc.ListPaymentsRequest)
			if !ok {
				return nil, er.New("Argument is not a ListPaymentsRequest")
			}

			//	invoke Lightning list payments command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var listPaymentsResp *lnrpc.ListPaymentsResponse

				listPaymentsResp, err := cc.ListPayments(context.TODO(), listPaymentsReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return listPaymentsResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service queryroutes
	{
		path: "/api/v1/payment/queryroutes",
		req:  (*lnrpc.QueryRoutesRequest)(nil),
		res:  (*lnrpc.QueryRoutesResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			queryRoutesReq, ok := m.(*lnrpc.QueryRoutesRequest)
			if !ok {
				return nil, er.New("Argument is not a QueryRoutesRequest")
			}

			//	invoke Lightning query routes command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var queryRoutesResp *lnrpc.QueryRoutesResponse

				queryRoutesResp, err := cc.QueryRoutes(context.TODO(), queryRoutesReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return queryRoutesResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service fwdinghistory
	{
		path: "/api/v1/payment/fwdinghistory",
		req:  (*lnrpc.ForwardingHistoryRequest)(nil),
		res:  (*lnrpc.ForwardingHistoryResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			forwardingHistoryReq, ok := m.(*lnrpc.ForwardingHistoryRequest)
			if !ok {
				return nil, er.New("Argument is not a ForwardingHistoryRequest")
			}

			//	invoke Lightning forwarding history command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var forwardingHistoryResp *lnrpc.ForwardingHistoryResponse

				forwardingHistoryResp, err := cc.ForwardingHistory(context.TODO(), forwardingHistoryReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return forwardingHistoryResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service querymc
	{
		path: "/api/v1/payment/querymc",
		req:  nil,
		res:  (*routerrpc.QueryMissionControlResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	query the mission control status
			cc, errr := c.withRouterServer()
			if cc != nil {
				var queryMissionControlResp *routerrpc.QueryMissionControlResponse

				queryMissionControlResp, err := cc.QueryMissionControl(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return queryMissionControlResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service queryprob
	{
		path: "/api/v1/payment/queryprob",
		req:  (*routerrpc.QueryProbabilityRequest)(nil),
		res:  (*routerrpc.QueryProbabilityResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			queryProbabilityReq, ok := m.(*routerrpc.QueryProbabilityRequest)
			if !ok {
				return nil, er.New("Argument is not a QueryProbabilityRequest")
			}

			//	invoke the probability service
			cc, errr := c.withRouterServer()
			if cc != nil {
				var queryProbabilityResp *routerrpc.QueryProbabilityResponse

				queryProbabilityResp, err := cc.QueryProbability(context.TODO(), queryProbabilityReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return queryProbabilityResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service resetmc
	{
		path: "/api/v1/payment/resetmc",
		req:  nil,
		res:  (*routerrpc.ResetMissionControlResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	invoke reset mission controle service
			cc, errr := c.withRouterServer()
			if cc != nil {
				var resetMissionControlResp *routerrpc.ResetMissionControlResponse

				resetMissionControlResp, err := cc.ResetMissionControl(context.TODO(), nil)
				if err != nil {
					return nil, er.E(err)
				} else {
					return resetMissionControlResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service buildroute
	{
		path: "/api/v1/payment/buildroute",
		req:  (*routerrpc.BuildRouteRequest)(nil),
		res:  (*routerrpc.BuildRouteResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			buildRouteReq, ok := m.(*routerrpc.BuildRouteRequest)
			if !ok {
				return nil, er.New("Argument is not a BuildRouteRequest")
			}

			//	invoke reset mission controle service
			cc, errr := c.withRouterServer()
			if cc != nil {
				var buildRouteResp *routerrpc.BuildRouteResponse

				buildRouteResp, err := cc.BuildRoute(context.TODO(), buildRouteReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return buildRouteResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service connect
	{
		path: "/api/v1/peer/connect",
		req:  (*lnrpc.ConnectPeerRequest)(nil),
		res:  (*lnrpc.ConnectPeerResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			connectPeerReq, ok := m.(*lnrpc.ConnectPeerRequest)
			if !ok {
				return nil, er.New("Argument is not a ConnectPeerRequest")
			}

			//	invoke Lightning connect peer command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var connectPeerResp *lnrpc.ConnectPeerResponse

				connectPeerResp, err := cc.ConnectPeer(context.TODO(), connectPeerReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return connectPeerResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service disconnect
	{
		path: "/api/v1/peer/disconnect",
		req:  (*lnrpc.DisconnectPeerRequest)(nil),
		res:  (*lnrpc.DisconnectPeerResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			//	get the request payload
			disconnectPeerReq, ok := m.(*lnrpc.DisconnectPeerRequest)
			if !ok {
				return nil, er.New("Argument is not a DisconnectPeerRequest")
			}

			//	invoke Lightning disconnect peer command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var disconnectPeerResp *lnrpc.DisconnectPeerResponse

				disconnectPeerResp, err := cc.DisconnectPeer(context.TODO(), disconnectPeerReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return disconnectPeerResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
	//	service listpeers
	{
		path: "/api/v1/peer",
		req:  nil,
		res:  (*lnrpc.ListPeersResponse)(nil),
		f: func(c *RpcContext, m proto.Message) (proto.Message, er.R) {

			listPeersReq := &lnrpc.ListPeersRequest{
				LatestError: true,
			}

			//	invoke Lightning list peers command
			cc, errr := c.withRpcServer()
			if cc != nil {
				var listPeersResp *lnrpc.ListPeersResponse

				listPeersResp, err := cc.ListPeers(context.TODO(), listPeersReq)
				if err != nil {
					return nil, er.E(err)
				} else {
					return listPeersResp, nil
				}
			} else {
				return nil, errr
			}
		},
	},
}

type RpcContext struct {
	MaybeCC             *chainreg.ChainControl
	MaybeNeutrino       *neutrino.ChainService
	MaybeWallet         *wallet.Wallet
	MaybeRpcServer      lnrpc.LightningServer
	MaybeWalletUnlocker *walletunlocker.UnlockerService
	MaybeMetaService    lnrpc.MetaServiceServer
	MaybeVerRPCServer   verrpc.VersionerServer
	MaybeRouterServer   routerrpc.RouterServer
}

func with(thing interface{}, name string) er.R {
	if thing == nil {
		return er.Errorf("Could not call function because [%s] is not yet ready", name)
	}
	return nil
}
func (c *RpcContext) withCC() (*chainreg.ChainControl, er.R) {
	return c.MaybeCC, with(c.MaybeCC, "ChainController")
}
func (c *RpcContext) withNeutrino() (*neutrino.ChainService, er.R) {
	return c.MaybeNeutrino, with(c.MaybeNeutrino, "Neutrino")
}
func (c *RpcContext) withWallet() (*wallet.Wallet, er.R) {
	return c.MaybeWallet, with(c.MaybeWallet, "Wallet")
}
func (c *RpcContext) withRpcServer() (lnrpc.LightningServer, er.R) {
	return c.MaybeRpcServer, with(c.MaybeRpcServer, "LightningServer")
}
func (c *RpcContext) withUnlocker() (*walletunlocker.UnlockerService, er.R) {
	return c.MaybeWalletUnlocker, with(c.MaybeWalletUnlocker, "UnlockerService")
}
func (c *RpcContext) withMetaServer() (lnrpc.MetaServiceServer, er.R) {
	return c.MaybeMetaService, with(c.MaybeMetaService, "MetaServiceService")
}
func (c *RpcContext) withVerRPCServer() (verrpc.VersionerServer, er.R) {
	return c.MaybeVerRPCServer, with(c.MaybeVerRPCServer, "VersionerService")
}
func (c *RpcContext) withRouterServer() (routerrpc.RouterServer, er.R) {
	return c.MaybeRouterServer, with(c.MaybeRouterServer, "RouterServer")
}

type SimpleHandler struct {
	rf RpcFunc
	c  *RpcContext
}

func unmarshal1(r *http.Request, m proto.Message, isJson bool) er.R {
	if b, err := io.ReadAll(r.Body); err != nil {
		return er.E(err)
	} else if isJson {
		// Use jsoniter for unmarshaling because it is far more forgiving
		if err := jsoniter.Unmarshal(b, m); err != nil {
			return er.E(err)
		}
	} else if err := proto.Unmarshal(b, m); err != nil {
		return er.E(err)
	}
	return nil
}

func unmarshal(r *http.Request, m proto.Message, isJson bool) er.R {
	if isJson {
		if err := jsonpb.Unmarshal(r.Body, m); err != nil {
			return er.E(err)
		}
	} else {
		if b, err := io.ReadAll(r.Body); err != nil {
			return er.E(err)
		} else if err := proto.Unmarshal(b, m); err != nil {
			return er.E(err)
		}
	}
	return nil
}
func marshal(w http.ResponseWriter, m proto.Message, isJson bool) er.R {
	if m == nil {
		return nil
	}
	if isJson {
		marshaler := jsonpb.Marshaler{
			OrigName:     false,
			EnumsAsInts:  false,
			EmitDefaults: true,
			Indent:       "\t",
		}
		if s, err := marshaler.MarshalToString(m); err != nil {
			return er.E(err)
		} else if _, err := io.WriteString(w, s); err != nil {
			return er.E(err)
		}
	} else {
		if b, err := proto.Marshal(m); err != nil {
			return er.E(err)
		} else if _, err := w.Write(b); err != nil {
			return er.E(err)
		}
	}
	return nil
}

func (s *SimpleHandler) ServeHttpOrErr(w http.ResponseWriter, r *http.Request, isJson bool) er.R {
	var req proto.Message
	if s.rf.req != nil {
		if r.Method != "POST" {
			return er.New("Request should be a POST because the endpoint requires input")
		}
		req1 := reflect.New(reflect.TypeOf(s.rf.req).Elem())
		if r, ok := req1.Interface().(proto.Message); !ok {
			panic("elem is not a proto.Message")
		} else {
			req = r
		}
		if err := unmarshal(r, req, isJson); err != nil {
			return err
		}
	} else if r.Method != "GET" {
		return er.New("Request should be a GET because the endpoint requires no input")
	}
	if res, err := s.rf.f(s.c, req); err != nil {
		return err
	} else if err := marshal(w, res, isJson); err != nil {
		return err
	}
	return nil
}

func (s *SimpleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ct := strings.ToLower(r.Header.Get("Content-Type"))
	isJson := strings.Contains(ct, "application/json")
	if !isJson && !strings.Contains(ct, "application/protobuf") {
		if r.Method == "GET" {
			isJson = true
		} else {
			w.Header().Set("Connection", "close")
			w.Header().Set("Content-Type", "text/plain")
			http.Error(w, "400 - Invalid content type, must be json or protobuf", http.StatusBadRequest)
			return
		}
	}
	if err := s.ServeHttpOrErr(w, r, isJson); err != nil {
		if err = marshal(w, &lnrpc.RestError{
			Message: err.Message(),
			Stack:   err.Stack(),
		}, isJson); err != nil {
			log.Errorf("Error replying to request for [%s] from [%s] - error sending error, giving up: [%s]",
				r.RequestURI, r.RemoteAddr, err)
		}
	}
}

func RestHandlers(c *RpcContext) *mux.Router {
	r := mux.NewRouter()
	for _, rf := range rpcFunctions {
		r.Handle(rf.path, &SimpleHandler{c: c, rf: rf})
	}
	return r
}
