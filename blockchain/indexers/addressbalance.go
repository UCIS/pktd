// Copyright (c) 2023 Caleb James DeLisle
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"github.com/pkt-cash/pktd/blockchain"
	"github.com/pkt-cash/pktd/blockchain/addressbalance"
	"github.com/pkt-cash/pktd/btcutil"
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/btcutil/util/tmap"
	"github.com/pkt-cash/pktd/chaincfg"
	"github.com/pkt-cash/pktd/database"
	"github.com/pkt-cash/pktd/pktlog/log"
	"github.com/pkt-cash/pktd/txscript"
)

type AddressBalanceIndex struct {
	syncToHeight uint32
	db           database.DB
}

var _ Indexer = (*AddressBalanceIndex)(nil)

func (abi *AddressBalanceIndex) Key() []byte {
	return addressbalance.Key()
}

func (abi *AddressBalanceIndex) Name() string {
	return "address balance index"
}

func (abi *AddressBalanceIndex) Create(dbTx database.Tx) er.R {
	if h, err := addressbalance.Create(dbTx); err != nil {
		return err
	} else {
		abi.syncToHeight = h
	}
	return nil
}

var followScript addressbalance.BalanceChange

func (abi *AddressBalanceIndex) Init() er.R {
	if addr, e := btcutil.DecodeAddress("pGsZXFt5d7WZhgWbXTY1VtfdicfCJ9Q3Hs", &chaincfg.PktMainNetParams); e != nil {
		panic(e)
	} else if addr, e := txscript.PayToAddrScript(addr); e != nil {
		panic(e)
	} else {
		followScript = addressbalance.BalanceChange{
			AddressScr: addr,
			Diff:       0,
		}
	}
	return abi.db.Update(func(tx database.Tx) er.R {
		return abi.Create(tx)
	})
}

func getBlockChanges(
	block *btcutil.Block,
	spent []blockchain.SpentTxOut,
) *tmap.Map[addressbalance.BalanceChange, struct{}] {
	outCount := 0
	for _, tx := range block.Transactions() {
		outCount += len(tx.MsgTx().TxOut)
	}
	bcs := addressbalance.NewBalanceChanges()
	insert := func(bc *addressbalance.BalanceChange) {
		if old, _ := tmap.Insert(bcs, bc, &struct{}{}); old != nil {
			bc.Diff += old.Diff
		}
	}
	for _, tx := range block.Transactions() {
		for _, out := range tx.MsgTx().TxOut {
			insert(&addressbalance.BalanceChange{
				AddressScr: out.PkScript,
				Diff:       out.Value,
			})
		}
	}
	for _, sp := range spent {
		insert(&addressbalance.BalanceChange{
			AddressScr: sp.PkScript,
			Diff:       -sp.Amount,
		})
	}
	return bcs
}

func (abi *AddressBalanceIndex) ConnectBlock(
	tx database.Tx,
	block *btcutil.Block,
	spent []blockchain.SpentTxOut,
) er.R {
	bc := getBlockChanges(block, spent)
	if err := addressbalance.UpdateBalances(tx, uint32(block.Height()), bc); err != nil {
		return err
	}
	abi.syncToHeight = uint32(block.Height())
	return nil
}

func (abi *AddressBalanceIndex) DisconnectBlock(
	tx database.Tx,
	block *btcutil.Block,
	spent []blockchain.SpentTxOut,
) er.R {
	bc := getBlockChanges(block, spent)
	// Invert everything since we're removing the block
	tmap.ForEach(bc, func(k *addressbalance.BalanceChange, v *struct{}) er.R {
		k.Diff = -k.Diff
		return nil
	})
	if key, _ := tmap.GetEntry(bc, &followScript); key != nil {
		log.Debugf("Address pGsZXFt5d7WZhgWbXTY1VtfdicfCJ9Q3Hs changed by [%d] in block [%d]",
			key.Diff, block.Height())
	}
	if err := addressbalance.UpdateBalances(tx, uint32(block.Height())-1, bc); err != nil {
		return err
	}
	abi.syncToHeight = uint32(block.Height()) - 1
	return nil
}

func NewAddressBalances(db database.DB) *AddressBalanceIndex {
	return &AddressBalanceIndex{
		db:           db,
		syncToHeight: 0,
	}
}
