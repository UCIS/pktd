package votes

import (
	"encoding/binary"

	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/database"
	"github.com/pkt-cash/pktd/pktlog/log"
)

// votesBucketName is where we put the votes cast using the v2.0 election
// system. The key is the big endian representation of the block number
// and the value is the protobuf pkt_pb.BlockVotes structure.
var votesBucketName = []byte("votes")

type NsVote struct {
	VoterPkScript           []byte
	VoterIsWillingCandidate bool
	VoteCastInBlock         uint32
	VoteForPkScript         []byte
}

func dbGetVotes(dbTx database.Tx, startBlock uint32, handler func(*NsVote) er.R) er.R {
	buck := dbTx.Metadata().Bucket(votesBucketName)
	sb := [4]byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(sb[:], startBlock)
	c := buck.Cursor()
	for c.Seek(sb[:]); c.Next(); {
		if v, err := decodeVote(c.Key(), c.Value()); err != nil {
			return err
		} else if err := handler(v); err != nil {
			if err != er.LoopBreak {
				return err
			} else {
				return nil
			}
		}
	}
	return nil
}

// dbInsertBlockVotes puts voting information in the db for a block
// dbTx: A read/write transaction
// blockNum: The height of the block which we are storing data for
// votes: The vote info to store
func dbInsertBlockVotes(dbTx database.Tx, votes []NsVote) er.R {
	buck := dbTx.Metadata().Bucket(votesBucketName)
	for _, vote := range votes {
		k, v := encodeVote(vote)
		if err := buck.Put(k, v); err != nil {
			return err
		}
	}
	return nil
}

// dbPruneBlockVotes deletes all voting information for blocks of height greater
// than or equal to blockNum. Used when rolling back.
// dbTx: A read/write transaction
// blockNum: The height of the first block that should have vote info removed from the db
// Information for this block AND all blocks higher than this will be removed.
func dbPruneBlockVotes(dbTx database.Tx, blockNum uint32) er.R {
	buck := dbTx.Metadata().Bucket(votesBucketName)
	var from [4]byte
	binary.BigEndian.PutUint32(from[:], blockNum)
	cursor := buck.Cursor()
	for ok := cursor.Seek(from[:]); ok; ok = cursor.Next() {
		if err := cursor.Delete(); err != nil {
			return err
		}
	}
	return nil
}

func dbInitBlockVotes(dbTx database.Tx) (uint32, er.R) {
	buck := dbTx.Metadata().Bucket(votesBucketName)
	if buck == nil {
		log.Infof("Creating votes bucket in database")
		if b, err := dbTx.Metadata().CreateBucket(votesBucketName); err != nil {
			return 0, err
		} else {
			buck = b
		}
	}
	cursor := buck.Cursor()
	if !cursor.Last() {
		// No data in votes bucket yet, synced up to 0
		return 0, nil
	}
	k := cursor.Key()
	if len(k) != 4 {
		return 0, er.Errorf("Votes bucket contains invalid last key: [%v]", k)
	}
	return binary.BigEndian.Uint32(k), nil
}

func dbDestroyBlockVotes(dbTx database.Tx) er.R {
	buck := dbTx.Metadata().Bucket(votesBucketName)
	if buck == nil {
		return nil
	}
	log.Infof("Deleting votes bucket from database")
	return dbTx.Metadata().DeleteBucket(votesBucketName)
}
