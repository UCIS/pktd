package votes

import (
	"github.com/pkt-cash/pktd/blockchain"
	"github.com/pkt-cash/pktd/btcutil"
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/database"
	"github.com/pkt-cash/pktd/pktlog/log"
)

func Key() []byte {
	return votesBucketName
}

func Init(dbTx database.Tx) (uint32, er.R) {
	return dbInitBlockVotes(dbTx)
}

func Drop(db database.DB, interrupt <-chan struct{}) er.R {
	return db.Update(func(tx database.Tx) er.R {
		return dbDestroyBlockVotes(tx)
	})
}

func ConnectBlock(dbTx database.Tx, block *btcutil.Block, stxo []blockchain.SpentTxOut) er.R {
	votes, err := parseVotes(block, stxo)
	if err != nil {
		log.Errorf("Unable to parse votes from block number [%d]: [%s]", block.Height(), err)
		// Returning the error will cause a crash
		return err
	}
	if err = dbInsertBlockVotes(dbTx, votes); err != nil {
		log.Errorf("Unable to store votes from block number [%d]: [%s]", block.Height(), err)
		return err
	}
	return nil
}

func DisconnectBlock(dbTx database.Tx, blockNum uint32) er.R {
	return dbPruneBlockVotes(dbTx, blockNum)
}

func GetVotes(dbTx database.Tx, startBlock uint32, handler func(*NsVote) er.R) er.R {
	return dbGetVotes(dbTx, startBlock, handler)
}
