package addressbalance

import (
	"bytes"

	"github.com/pkt-cash/pktd/btcutil"
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/btcutil/util/tmap"
	"github.com/pkt-cash/pktd/chaincfg"
	"github.com/pkt-cash/pktd/database"
	"github.com/pkt-cash/pktd/pktlog/log"
	"github.com/pkt-cash/pktd/txscript"
)

// A change of balance of an address
type BalanceChange struct {
	// The address in pkScript format
	AddressScr []byte
	// The change of balance as a positive or negative number
	Diff int64
}

func Key() []byte {
	return balancesBucketName
}

func NewBalanceChanges() *tmap.Map[BalanceChange, struct{}] {
	return tmap.New[BalanceChange, struct{}](func(a, b *BalanceChange) int {
		return bytes.Compare(a.AddressScr, a.AddressScr)
	})
}

func UpdateBalances(
	dbTx database.Tx,
	blockNum uint32,
	changes *tmap.Map[BalanceChange, struct{}],
) er.R {
	scr := make([][]byte, 0, tmap.Len(changes))
	amts := make([]int64, 0, tmap.Len(changes))
	tmap.ForEach(changes, func(c *BalanceChange, _ *struct{}) er.R {
		addr := txscript.PkScriptToAddress(c.AddressScr, &chaincfg.PktMainNetParams)
		log.Debugf("Address [%s] changed by [%d] in block [%d]",
			addr.EncodeAddress(), c.Diff, blockNum)
		scr = append(scr, c.AddressScr)
		amts = append(amts, c.Diff)
		return nil
	})
	balances, err := dbFetchBalances(dbTx, scr)
	if err != nil {
		return err
	}
	for i := 0; i < len(amts); i++ {
		if err := applyBalanceChange(&balances[i], amts[i], blockNum); err != nil {
			return err
		}
	}
	return dbPutBalances(dbTx, balances)
}

func Create(dbTx database.Tx) (uint32, er.R) {
	return dbInitBalances(dbTx)
}

func Init(dbTx database.Tx) (uint32, er.R) {
	return dbInitBalances(dbTx)
}

func Drop(db database.DB, interrupt <-chan struct{}) er.R {
	return db.Update(func(tx database.Tx) er.R {
		return dbDestroyBalances(tx)
	})
}

type AddressBalance struct {
	AddressScript []byte
	Balance       btcutil.Amount
}

func GetBalances(
	dbTx database.Tx,
	epochNum uint32,
	startFrom []byte,
	handler func(*AddressBalance) er.R,
) er.R {
	heightLimit := epochLastBlock(epochNum)
	return dbListBalances(dbTx, startFrom, func(b *addressBalance) er.R {
		return handler(&AddressBalance{
			AddressScript: b.addressScript,
			Balance:       btcutil.Amount(parseBalance(b, heightLimit)),
		})
	})
}

func EpochNum(blockHeight int32) uint32 {
	return epochNum(uint32(blockHeight))
}
