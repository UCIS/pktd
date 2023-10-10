package addressbalance

import (
	"encoding/binary"
	"time"

	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/database"
	"github.com/pkt-cash/pktd/pktlog/log"
)

// balancesBucketName is where we track balances of every address with
// coins on it. The key is the address in pkScript format and the value is
// the pkt_pb.AddressBalances protobuf structure.
var balancesBucketName = []byte("addressbalance")

const balanceInfoLen = 4 + 8

type balanceInfo struct {
	// The balance info is valid as of this height
	blockNum uint32
	// The balance at this height
	balance uint64
}

func encodeBalanceInfo(bi []balanceInfo) []byte {
	out := make([]byte, len(bi)*balanceInfoLen)
	for i, b := range bi {
		idx := i * balanceInfoLen
		binary.LittleEndian.PutUint32(out[idx:idx+4], b.blockNum)
		binary.LittleEndian.PutUint64(out[idx+4:idx+4+8], b.balance)
	}
	return out
}

func decodeBalanceInfo(b []byte) ([]balanceInfo, er.R) {
	out := make([]balanceInfo, 0, len(b)/balanceInfoLen)
	for i := 0; i < len(b); i += balanceInfoLen {
		sl := b[i : i+balanceInfoLen]
		if len(sl) < balanceInfoLen {
			return nil, er.Errorf("Failed to parse balanceInfo, record length is [%d]", len(b))
		}
		out = append(out, balanceInfo{
			blockNum: binary.LittleEndian.Uint32(sl[:4]),
			balance:  binary.LittleEndian.Uint64(sl[4:]),
		})
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// The addressbalances bucket stores current and recent snapshot of address balances.
// The keys in this bucket are the addressScript and the values are the serialized
// balanceInfo.
// -----------------------------------------------------------------------------
type addressBalance struct {
	// The address whose balance we are considering (in pkScript format)
	addressScript []byte
	// The balance info for this address, if the address does not exist on chain
	// or the address has zero balance and has been pruned since the last checkpoint,
	// then this field will be nil
	balanceInfo []balanceInfo
}

// dbFetchBalances gets the balance info for a list of addresses
// dbTx: A read or read/write db transactin
// addressScripts: A list of addresses in pkScript form
// returns: A list of addressBalance entries for each address
func dbFetchBalances(dbTx database.Tx, addressScripts [][]byte) ([]addressBalance, er.R) {
	balancesBucket := dbTx.Metadata().Bucket(balancesBucketName)
	out := make([]addressBalance, 0, len(addressScripts))
	for _, addressScript := range addressScripts {
		balances := balancesBucket.Get(addressScript)
		if balances != nil {
			if bi, err := decodeBalanceInfo(balances); err != nil {
				return nil, err
			} else {
				out = append(out, addressBalance{addressScript, bi})
			}
		} else {
			out = append(out, addressBalance{addressScript, nil})
		}
	}
	return out, nil
}

// dbPutBalances stores a list of address balances.
// dbTx: A read/write transaction
// balances: A list of addressBalance objects
func dbPutBalances(dbTx database.Tx, balances []addressBalance) er.R {
	balancesBucket := dbTx.Metadata().Bucket(balancesBucketName)
	for _, bal := range balances {
		if bal.balanceInfo == nil {
			if err := balancesBucket.Delete(bal.addressScript); err != nil {
				return err
			}
		} else {
			balancesBucket.Put(bal.addressScript, encodeBalanceInfo(bal.balanceInfo))
		}
	}
	return nil
}

func dbInitBalances(dbTx database.Tx) (uint32, er.R) {
	buck := dbTx.Metadata().Bucket(balancesBucketName)
	if buck == nil {
		log.Infof("Creating address balances in database")
		if b, err := dbTx.Metadata().CreateBucket(balancesBucketName); err != nil {
			return 0, err
		} else {
			buck = b
		}
	}
	t0 := time.Now()
	maxBlock := uint32(0)
	if err := buck.ForEach(func(_, v []byte) er.R {
		if bi, err := decodeBalanceInfo(v); err != nil {
			return err
		} else {
			for _, b := range bi {
				if b.blockNum > maxBlock {
					maxBlock = b.blockNum
				}
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}
	log.Infof("Scanned address balances in [%d] milliseconds", time.Since(t0).Milliseconds())
	return maxBlock, nil
}

func dbDestroyBalances(dbTx database.Tx) er.R {
	buck := dbTx.Metadata().Bucket(balancesBucketName)
	if buck == nil {
		return nil
	}
	log.Infof("Deleting address balances from database")
	return dbTx.Metadata().DeleteBucket(balancesBucketName)
}

func dbListBalances(dbTx database.Tx, startFrom []byte, handler func(*addressBalance) er.R) er.R {
	buck := dbTx.Metadata().Bucket(balancesBucketName)
	if buck == nil {
		return er.Errorf("Address balances not indexed, --addressbalances required for this RPC")
	}
	c := buck.Cursor()
	if startFrom != nil {
		c.Seek(startFrom)
	}
	i := 0
	for c.Next() {
		if bi, err := decodeBalanceInfo(c.Value()); err != nil {
			return err
		} else if err := handler(&addressBalance{
			addressScript: c.Key(),
			balanceInfo:   bi,
		}); err != nil {
			return err
		}
		i++
	}
	return nil
}
