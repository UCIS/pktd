// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/emirpasic/gods/trees/redblacktree"
	"github.com/emirpasic/gods/utils"
	"github.com/pkt-cash/pktd/btcec"
	"github.com/pkt-cash/pktd/btcutil"
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/pktlog/log"
	"github.com/pkt-cash/pktd/pktwallet/waddrmgr"
	"github.com/pkt-cash/pktd/pktwallet/wallet/enough"
	"github.com/pkt-cash/pktd/pktwallet/wallet/txauthor"
	"github.com/pkt-cash/pktd/pktwallet/wallet/txrules"
	"github.com/pkt-cash/pktd/pktwallet/walletdb"
	"github.com/pkt-cash/pktd/pktwallet/wtxmgr/dbstructs"
	"github.com/pkt-cash/pktd/pktwallet/wtxmgr/unspent"
	"github.com/pkt-cash/pktd/txscript"
	"github.com/pkt-cash/pktd/wire"
)

// Maximum number of inputs which will be included in a transaction
const MaxInputsPerTx = 1460

// Maximum number of inputs which can be included in a transaction if there is
// at least one legacy non-segwit input
const MaxInputsPerTxLegacy = 499

var InsufficientFundsError = er.GenericErrorType.CodeWithDetail("InsufficientFundsError",
	"insufficient funds available to construct transaction")

var TooManyInputsError = er.GenericErrorType.CodeWithDetail("TooManyInputsError",
	"unable to construct transaction because there are too many inputs, you may need to fold coins")

var UnconfirmedCoinsError = er.GenericErrorType.CodeWithDetail("UnconfirmedCoinsError",
	"unable to construct transaction, there are coins but they are not yet confirmed")

func makeInputSource(eligible []*dbstructs.Unspent) txauthor.InputSource {
	// Current inputs and their total value.  These are closed over by the
	// returned input source and reused across multiple calls.
	currentTotal := btcutil.Amount(0)
	currentInputs := make([]*wire.TxIn, 0, len(eligible))
	currentAdditonal := make([]wire.TxInAdditional, 0, len(eligible))

	return func(target btcutil.Amount) (btcutil.Amount, []*wire.TxIn, []wire.TxInAdditional, er.R) {
		for currentTotal < target && len(eligible) != 0 {
			nextCredit := eligible[0]
			eligible = eligible[1:]
			nextInput := wire.NewTxIn(&nextCredit.OutPoint, nil, nil)
			currentTotal += btcutil.Amount(nextCredit.Value)
			currentInputs = append(currentInputs, nextInput)
			v := nextCredit.Value
			currentAdditonal = append(currentAdditonal, wire.TxInAdditional{
				PkScript: nextCredit.PkScript,
				Value:    &v,
			})
		}
		return currentTotal, currentInputs, currentAdditonal, nil
	}
}

// secretSource is an implementation of txauthor.SecretSource for the wallet's
// address manager.
type secretSource struct {
	*waddrmgr.Manager
	addrmgrNs walletdb.ReadBucket
}

func (s secretSource) GetKey(addr btcutil.Address) (*btcec.PrivateKey, bool, er.R) {
	ma, err := s.Address(s.addrmgrNs, addr)
	if err != nil {
		return nil, false, err
	}

	mpka, ok := ma.(waddrmgr.ManagedPubKeyAddress)
	if !ok {
		e := er.Errorf("managed address type for %v is `%T` but "+
			"want waddrmgr.ManagedPubKeyAddress", addr, ma)
		return nil, false, e
	}
	privKey, err := mpka.PrivKey()
	if err != nil {
		return nil, false, err
	}
	return privKey, ma.Compressed(), nil
}

func (s secretSource) GetScript(addr btcutil.Address) ([]byte, er.R) {
	ma, err := s.Address(s.addrmgrNs, addr)
	if err != nil {
		return nil, err
	}

	msa, ok := ma.(waddrmgr.ManagedScriptAddress)
	if !ok {
		e := er.Errorf("managed address type for %v is `%T` but "+
			"want waddrmgr.ManagedScriptAddress", addr, ma)
		return nil, e
	}
	return msa.Script()
}

// txToOutputs creates a signed transaction which includes each output from
// outputs.  Previous outputs to reedeem are chosen from the passed account's
// UTXO set and minconf policy. An additional output may be added to return
// change to the wallet.  An appropriate fee is included based on the wallet's
// current relay fee.  The wallet must be unlocked to create the transaction.
//
// NOTE: The dryRun argument can be set true to create a tx that doesn't alter
// the database. A tx created with this set to true will intentionally have no
// input scripts added and SHOULD NOT be broadcasted.
func (w *Wallet) txToOutputs(txr CreateTxReq) (tx *txauthor.AuthoredTx, err er.R) {

	chainClient, err := w.requireChainClient()
	if err != nil {
		return nil, err
	}

	dbtx, err := w.db.BeginReadWriteTx()
	if err != nil {
		return nil, err
	}
	defer dbtx.Rollback()

	addrmgrNs := dbtx.ReadWriteBucket(waddrmgrNamespaceKey)

	// Get current block's height and hash.
	bs, err := chainClient.BestBlock()
	if err != nil {
		return nil, err
	}

	isEnough := enough.MkIsEnough(txr.Outputs, txr.FeeSatPerKB)
	t0 := time.Now()
	eligibleOuts, visits, err := w.findEligibleOutputs(
		dbtx, isEnough, txr.InputAddresses, txr.Minconf, bs,
		txr.InputMinHeight, txr.InputComparator, txr.MaxInputs)
	if err != nil {
		return nil, err
	}
	log.Infof("findEligibleOutputs() completed in [%s], visited [%d] utxos",
		time.Since(t0).String(), visits)

	addrStr := "<all>"
	if len(txr.InputAddresses) > 0 {
		addrs := make([]string, 0, len(txr.InputAddresses))
		for _, a := range txr.InputAddresses {
			addrs = append(addrs, a.EncodeAddress())
		}
		addrStr = strings.Join(addrs, ", ")
	}
	log.Debugf("Found [%d] eligable inputs from addresses [%s], excluded [%d] (unconfirmed) "+
		"and [%d] (too many inputs for tx)",
		len(eligibleOuts.credits), addrStr, eligibleOuts.unconfirmedCount, eligibleOuts.unusedCount)
	for _, eo := range eligibleOuts.credits {
		log.Debugf("  %s @ %d - %s", eo.OutPoint.String(), eo.Block.Height, btcutil.Amount(eo.Value).String())
	}

	inputSource := makeInputSource(eligibleOuts.credits)
	changeSource := func() ([]byte, er.R) {
		// Derive the change output script.  As a hack to allow
		// spending from the imported account, change addresses are
		// created from account 0.
		var changeAddr btcutil.Address
		var err er.R
		if txr.ChangeAddress != nil {
			changeAddr = *txr.ChangeAddress
		} else {
			for _, c := range eligibleOuts.credits {
				_, addrs, _, _ := txscript.ExtractPkScriptAddrs(c.PkScript, w.chainParams)
				if len(addrs) == 1 {
					changeAddr = addrs[0]
				}
			}
			if changeAddr == nil {
				err = er.New("Unable to find qualifying change address")
			}
		}
		if err != nil {
			return nil, err
		}
		return txscript.PayToAddrScript(changeAddr)
	}
	tx, err = txauthor.NewUnsignedTransaction(
		txr.Outputs, txr.FeeSatPerKB, inputSource, changeSource, txr.MaxInputs > -1)
	if err != nil {
		if !txauthor.ImpossibleTxError.Is(err) {
			return nil, err
		} else if eligibleOuts.unusedCount > 0 {
			return nil, TooManyInputsError.New(
				fmt.Sprintf("additional [%d] transactions containing [%f] coins",
					eligibleOuts.unusedCount, eligibleOuts.unusedAmt.ToBTC()), err)
		} else if eligibleOuts.unconfirmedCount > 0 {
			return nil, UnconfirmedCoinsError.New(
				fmt.Sprintf("there are [%f] coins available in [%d] unconfirmed transactions, "+
					"to spend from these you need to specify minconf=0",
					eligibleOuts.unconfirmedAmt.ToBTC(), eligibleOuts.unconfirmedCount), err)
		} else {
			if txr.InputAddresses != nil {
				return nil, InsufficientFundsError.New(
					fmt.Sprintf("address(es) [%s] do not have enough balance", addrStr), err)
			} else {
				return nil, InsufficientFundsError.New("wallet does not have enough balance", err)
			}
		}
	}

	// Randomize change position, if change exists, before signing.  This
	// doesn't affect the serialize size, so the change amount will still
	// be valid.
	if tx.ChangeIndex >= 0 {
		tx.RandomizeChangePosition()
	}

	// If a dry run was requested, we return now before adding the input
	// scripts, and don't commit the database transaction. The DB will be
	// rolled back when this method returns to ensure the dry run didn't
	// alter the DB in any way.
	if txr.SendMode == SendModeUnsigned {
		if err := dbtx.Commit(); err != nil {
			return nil, err
		}
		return tx, nil
	}

	err = tx.AddAllInputScripts(secretSource{w.Manager, addrmgrNs})
	if err != nil {
		return nil, err
	}

	err = validateMsgTx1(tx.Tx)
	if err != nil {
		return nil, err
	}

	if txr.SendMode != SendModeBcasted {
		return tx, nil
	}

	if err := dbtx.Commit(); err != nil {
		return nil, err
	}

	// Finally, we'll request the backend to notify us of the transaction
	// that pays to the change address, if there is one, when it confirms.
	if tx.ChangeIndex >= 0 {
		changePkScript := tx.Tx.TxOut[tx.ChangeIndex].PkScript
		_, addrs, _, err := txscript.ExtractPkScriptAddrs(
			changePkScript, w.chainParams,
		)
		if err != nil {
			return nil, err
		}
		w.watch.WatchAddrs(addrs)
	}

	return tx, nil
}

type amountCount struct {
	// Amount of coins
	amount btcutil.Amount

	isSegwit bool

	credits *redblacktree.Tree
}

func (a *amountCount) overLimit(maxInputs int) bool {
	count := a.credits.Size()
	if maxInputs > 0 {
		return count > maxInputs
	} else if count < MaxInputsPerTxLegacy {
	} else if a.isSegwit && count < MaxInputsPerTx {
	} else {
		return true
	}
	return false
}

// NilComparator compares by txid/index in order to make the red-black tree functions
func NilComparator(a, b interface{}) int {
	s1 := a.(*dbstructs.Unspent)
	if s1 == nil {
		panic("NilComparator: s1 == nil")
	}
	s2 := b.(*dbstructs.Unspent)
	if s2 == nil {
		panic("NilComparator: s2 == nil")
	}
	utils.Int64Comparator(int64(s1.Value), int64(s2.Value))
	txidCmp := bytes.Compare(s1.OutPoint.Hash[:], s2.OutPoint.Hash[:])
	if txidCmp != 0 {
		return txidCmp
	}
	return utils.UInt32Comparator(s1.OutPoint.Index, s2.OutPoint.Index)
}

// PreferOldest prefers oldest outputs first
func PreferOldest(a, b interface{}) int {
	s1 := a.(*dbstructs.Unspent)
	if s1 == nil {
		panic("PreferOldest: s1 == nil")
	}
	s2 := b.(*dbstructs.Unspent)
	if s2 == nil {
		panic("PreferOldest: s2 == nil")
	}

	if s1.Block.Height < s2.Block.Height {
		return -1
	} else if s1.Block.Height > s2.Block.Height {
		return 1
	} else {
		return NilComparator(s1, s2)
	}
}

// PreferNewest prefers newest outputs first
// func PreferNewest(a, b interface{}) int {
// 	return -PreferOldest(a, b)
// }

// PreferBiggest prefers biggest (coin value) outputs first
func PreferBiggest(a, b interface{}) int {
	s1 := a.(*dbstructs.Unspent)
	if s1 == nil {
		panic("PreferBiggest: s1 == nil")
	}
	s2 := b.(*dbstructs.Unspent)
	if s2 == nil {
		panic("PreferBiggest: s2 == nil")
	}

	if s1.Value < s2.Value {
		return 1
	} else if s1.Value > s2.Value {
		return -1
	} else {
		return NilComparator(s1, s2)
	}
}

// PreferSmallest prefers smallest (coin value) outputs first (spend the dust)
// func PreferSmallest(a, b interface{}) int {
// 	return -PreferBiggest(a, b)
// }

func convertResult(ac *amountCount) []*dbstructs.Unspent {
	ifaces := ac.credits.Keys()
	out := make([]*dbstructs.Unspent, len(ifaces))
	for i := range ifaces {
		out[i] = ifaces[i].(*dbstructs.Unspent)
		if out[i] == nil {
			panic("convertResult: out == nil")
		}
	}
	return out
}

type eligibleOutputs struct {
	credits          []*dbstructs.Unspent
	unconfirmedCount int
	unconfirmedAmt   btcutil.Amount
	unusedCount      int
	unusedAmt        btcutil.Amount
}

func (w *Wallet) findEligibleOutputs(
	dbtx walletdb.ReadWriteTx,
	isEnough enough.IsEnough,
	fromAddresses []btcutil.Address,
	minconf int32,
	bs *waddrmgr.BlockStamp,
	inputMinHeight int,
	inputComparator utils.Comparator,
	maxInputs int,
) (eligibleOutputs, int, er.R) {
	out := eligibleOutputs{}
	txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)

	haveAmounts := make(map[string]*amountCount)
	var winner *amountCount

	var outputsToDelete []wire.OutPoint
	burnedOutputCount := 0

	log.Debugf("Looking for unspents to build transaction")

	addrStrs := make(map[string]struct{})
	for _, a := range fromAddresses {
		addrStrs[a.String()] = struct{}{}
	}

	var err er.R
	var visits int
	if visits, err = w.TxStore.ForEachUnspentOutput(txmgrNs, nil, addrStrs, func(key []byte, uns *dbstructs.Unspent) er.R {

		if uns.Block.Height >= 0 && uns.Block.Height < int32(inputMinHeight) {
			log.Debugf("Skipping output %s at height %d because it is below minimum %d",
				uns.OutPoint.String(), uns.Block.Height, inputMinHeight)
			return nil
		}

		if uns.FromCoinBase {
			if !confirmed(int32(w.chainParams.CoinbaseMaturity), uns.Block.Height, bs.Height) {
				log.Debugf("Skipping immature coinbase output [%s] at height %d",
					uns.OutPoint.String(), uns.Block.Height)
				return nil
			} else if txrules.IsBurned(uns, w.chainParams, bs.Height+1440) {
				log.Tracef("Skipping burned output at height %d", uns.Block.Height)
				if len(outputsToDelete) < 1_000_000 {
					outputsToDelete = append(outputsToDelete, uns.OutPoint)
					burnedOutputCount++
				}
				return nil
			}
		}
		if uns.Value == 0 {
			log.Tracef("Skipping zero value output at height [%d]", uns.Block.Height)
			if len(outputsToDelete) < 1_000_000 {
				outputsToDelete = append(outputsToDelete, uns.OutPoint)
			}
			return nil
		}

		if minconf > 0 {
			// Only include this output if it meets the required number of
			// confirmations.  Coinbase transactions must have have reached
			// maturity before their outputs may be spent.
			if !confirmed(minconf, uns.Block.Height, bs.Height) {
				log.Debugf("Skipping unconfirmed output [%s] at height %d [cur height: %d]",
					uns.OutPoint.String(), uns.Block.Height, bs.Height)
				out.unconfirmedCount++
				out.unconfirmedAmt += btcutil.Amount(uns.Value)
				return nil
			}
		}

		// Locked unspent outputs are skipped.
		if w.LockedOutpoint(uns.OutPoint) {
			return nil
		}

		ha := haveAmounts[uns.Address]
		if ha == nil {
			haa := amountCount{}
			if inputComparator == nil {
				// If the user does not specify a comparator, we use the preferBiggest
				// comparator to prefer high value outputs over less valuable outputs.
				//
				// Without this, there would be a risk that the wallet collected a bunch
				// of dust and then - using arbitrary ordering - could not remove the dust
				// inputs to ever make the transaction small enough, despite having large
				// spendable outputs.
				//
				// This does NOT cause the default behavior of the wallet to prefer large
				// outputs over small, because with no explicit comparator, we short circuit
				// as soon as we have enough money to make the transaction.
				haa.credits = redblacktree.NewWith(PreferBiggest)
			} else {
				haa.credits = redblacktree.NewWith(inputComparator)
			}
			if addr, err := btcutil.DecodeAddress(uns.Address, w.chainParams); err != nil {
				log.Warnf("Unable to decode address [%s] from utxo [%s]", uns.Address, uns.OutPoint.String())
			} else {
				haa.isSegwit = addr.IsSegwit()
			}
			ha = &haa
			haveAmounts[uns.Address] = ha
		}
		ha.credits.Put(uns, nil)
		ha.amount += btcutil.Amount(uns.Value)
		if isEnough.WellIsIt(ha.credits.Size(), ha.isSegwit, ha.amount) {
			worst := ha.credits.Right().Key.(*dbstructs.Unspent)
			if worst == nil {
				panic("findEligibleOutputs: worst == nil")
			}
			if isEnough.WellIsIt(ha.credits.Size()-1, ha.isSegwit, ha.amount-btcutil.Amount(worst.Value)) {
				// Our amount is still fine even if we drop the worst credit
				// so we'll drop it and continue traversing to find the best outputs
				ha.credits.Remove(worst)
				ha.amount -= btcutil.Amount(worst.Value)
				out.unusedAmt += btcutil.Amount(worst.Value)
				out.unusedCount++
			}

			// If we have no explicit sorting specified then we can short-circuit
			// and avoid table-scanning the whole db
			if inputComparator == nil {
				winner = ha
				return er.LoopBreak
			}
		}

		if !ha.overLimit(maxInputs) {
			// We don't have too many inputs
		} else if isEnough.IsSweeping() && inputComparator == nil {
			// We're sweeping the wallet with no ordering specified
			// This means we should just short-circuit with a winner
			winner = ha
			return er.LoopBreak
		} else {
			// Too many inputs, we will remove the worst
			worst := ha.credits.Right().Key.(*dbstructs.Unspent)
			if worst == nil {
				panic("findEligibleOutputs: worst == nil")
			}
			ha.credits.Remove(worst)
			ha.amount -= btcutil.Amount(worst.Value)
			out.unusedAmt += btcutil.Amount(worst.Value)
			out.unusedCount++
		}
		return nil
	}); err != nil && !er.IsLoopBreak(err) {
		return out, visits, err
	}

	log.Debugf("Got unspents")

	if len(outputsToDelete) > 0 {
		wtxmgrBucket := dbtx.ReadWriteBucket(wtxmgrNamespaceKey)
		log.Infof("Deleting [%s] burned outputs and [%s] zero-value outputs",
			log.Int(burnedOutputCount), log.Int(len(outputsToDelete)-burnedOutputCount))
		for _, op := range outputsToDelete {
			if err := unspent.Delete(wtxmgrBucket, &op); err != nil {
				return out, visits, err
			}
		}
	}

	if inputComparator != nil {
		// This is a special consideration because when there is a custom comparator,
		// we don't short circuit early so we might have a winner on our hands but not
		// know it.
		for _, ac := range haveAmounts {
			if isEnough.WellIsIt(ac.credits.Size(), ac.isSegwit, ac.amount) {
				winner = ac
			}
		}
	}

	if winner != nil {
		// Easy path, we got enough in one address to win, we'll just return those
		for _, ac := range haveAmounts {
			if ac != winner {
				out.unusedAmt += ac.amount
				out.unusedCount += ac.credits.Size()
			}
		}
		out.credits = convertResult(winner)
		return out, visits, nil
	}

	// We don't have an easy answer with just one address, we need to get creative.
	// We will create a new tree using the preferBiggest in order to try to to get
	// a subset of inputs which fits inside of the required count
	outAc := amountCount{
		isSegwit: true,
		credits:  redblacktree.NewWith(PreferBiggest),
	}
	done := false
	for _, ac := range haveAmounts {
		if done {
			out.unusedAmt += ac.amount
			out.unusedCount += ac.credits.Size()
			continue
		}
		it := ac.credits.Iterator()
		for i := 0; it.Next(); i++ {
			outAc.credits.Put(it.Key(), nil)
		}
		outAc.isSegwit = outAc.isSegwit && ac.isSegwit

		wasOver := false
		for outAc.overLimit(maxInputs) {
			// Too many inputs, we will remove the worst
			worst := outAc.credits.Right().Key.(*dbstructs.Unspent)
			if worst == nil {
				panic("findEligibleOutputs: worst == nil")
			}
			outAc.credits.Remove(worst)
			outAc.amount -= btcutil.Amount(worst.Value)
			out.unusedAmt += btcutil.Amount(worst.Value)
			out.unusedCount++
			wasOver = true
		}
		if isEnough.IsSweeping() && !wasOver {
			// if we were never over the limit and we're sweeping multiple addresses,
			// lets go around and get another address
		} else if isEnough.WellIsIt(outAc.credits.Size(), outAc.isSegwit, outAc.amount) {
			// We have enough money to make the tx
			// We'll just iterate over the other entries to make unusedAmt and unusedCount correct
			done = true
		}
	}

	out.credits = convertResult(&outAc)
	return out, visits, nil
}

// addrMgrWithChangeSource returns the address manager bucket and a change
// source function that returns change addresses from said address manager.
func (w *Wallet) addrMgrWithChangeSource(dbtx walletdb.ReadWriteTx,
	account uint32) (walletdb.ReadWriteBucket, txauthor.ChangeSource) {

	addrmgrNs := dbtx.ReadWriteBucket(waddrmgrNamespaceKey)
	changeSource := func() ([]byte, er.R) {
		// Derive the change output script. We'll use the default key
		// scope responsible for P2WPKH addresses to do so. As a hack to
		// allow spending from the imported account, change addresses
		// are created from account 0.
		var changeAddr btcutil.Address
		var err er.R
		changeKeyScope := waddrmgr.KeyScopeBIP0084
		if account == waddrmgr.ImportedAddrAccount {
			changeAddr, _, err = w.newAddress(
				addrmgrNs, 0, changeKeyScope,
			)
		} else {
			changeAddr, _, err = w.newAddress(
				addrmgrNs, account, changeKeyScope,
			)
		}
		if err != nil {
			return nil, err
		}
		return txscript.PayToAddrScript(changeAddr)
	}
	return addrmgrNs, changeSource
}

// validateMsgTx1 verifies transaction input scripts for tx.  All previous output
// scripts from outputs redeemed by the transaction, in the same order they are
// spent, must be passed in the prevScripts slice.
func validateMsgTx1(tx *wire.MsgTx) er.R {
	hashCache := txscript.NewTxSigHashes(tx)
	if len(tx.Additional) != len(tx.TxIn) {
		return er.Errorf("len(tx.Additional) = [%d] but len(tx.TxIn) = [%d], cannot validate tx",
			len(tx.Additional), len(tx.TxIn))
	}
	for i, add := range tx.Additional {
		if len(add.PkScript) == 0 {
			return er.Errorf("Unable to validate transaction, add.PkScript is empty")
		} else if add.Value == nil {
			return er.Errorf("Unable to validate transaction, add.Value is unknown")
		}
		vm, err := txscript.NewEngine(add.PkScript, tx, i,
			txscript.StandardVerifyFlags, nil, hashCache, *add.Value)
		if err != nil {
			err.AddMessage("cannot create script engine")
			return err
		}
		err = vm.Execute()
		if err != nil {
			err.AddMessage("cannot validate transaction")
			return err
		}
	}
	return nil
}

// validateMsgTx verifies transaction input scripts for tx.  All previous output
// scripts from outputs redeemed by the transaction, in the same order they are
// spent, must be passed in the prevScripts slice.
func validateMsgTx(tx *wire.MsgTx, prevScripts [][]byte, inputValues []btcutil.Amount) er.R {
	add := make([]wire.TxInAdditional, 0, len(prevScripts))
	if len(prevScripts) != len(inputValues) {
		return er.Errorf("len(prevScripts) != len(inputValues)")
	}
	for i, ps := range prevScripts {
		v := int64(inputValues[i])
		add = append(add, wire.TxInAdditional{
			PkScript: ps,
			Value:    &v,
		})
	}
	return validateMsgTx1(&wire.MsgTx{
		Version:    tx.Version,
		TxIn:       tx.TxIn,
		TxOut:      tx.TxOut,
		LockTime:   tx.LockTime,
		Additional: add,
	})
}
