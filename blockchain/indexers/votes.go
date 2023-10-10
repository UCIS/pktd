// Copyright (c) 2023 Caleb James DeLisle
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"github.com/pkt-cash/pktd/blockchain"
	"github.com/pkt-cash/pktd/blockchain/votes"
	"github.com/pkt-cash/pktd/btcutil"
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/database"
)

type VotesIndex struct {
	syncToHeight uint32
	db           database.DB
}

var _ Indexer = (*VotesIndex)(nil)

func (vi *VotesIndex) Key() []byte {
	return votes.Key()
}

func (vi *VotesIndex) Name() string {
	return "vote table"
}

func (vi *VotesIndex) Create(dbTx database.Tx) er.R {
	if height, err := votes.Init(dbTx); err != nil {
		return err
	} else {
		vi.syncToHeight = height
	}
	return nil
}

func (vi *VotesIndex) Init() er.R {
	return vi.db.Update(func(tx database.Tx) er.R {
		return vi.Create(tx)
	})
}

func (vi *VotesIndex) ConnectBlock(dbTx database.Tx, block *btcutil.Block, stxo []blockchain.SpentTxOut) er.R {
	return votes.ConnectBlock(dbTx, block, stxo)
}

func (vi *VotesIndex) DisconnectBlock(dbTx database.Tx, block *btcutil.Block, stxo []blockchain.SpentTxOut) er.R {
	return votes.DisconnectBlock(dbTx, uint32(block.Height()))
}

func NewVotes(db database.DB) *VotesIndex {
	return &VotesIndex{
		db:           db,
		syncToHeight: 0,
	}
}
