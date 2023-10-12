package addressbalance

import (
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/chaincfg"
	"github.com/pkt-cash/pktd/pktlog/log"
	"github.com/pkt-cash/pktd/txscript"
)

const blocksPerEpoch = (60 * 24 * 7)

// An epoch is 1 week
func epochNum(blockHeight uint32) uint32 {
	return blockHeight / blocksPerEpoch
}

func epochLastBlock(epochNum uint32) uint32 {
	return (epochNum+1)*blocksPerEpoch - 1
}

func parseBalance(ab *addressBalance, heightLimit uint32) uint64 {
	for _, b := range ab.balanceInfo {
		if b.blockNum <= heightLimit {
			return b.balance
		}
	}
	return 0
}

// applyBalanceChange updates the balance value to take into account the change specified
// in change for a change of balance which takes place in the block number blockNum.
func applyBalanceChange(
	balance *addressBalance,
	change int64,
	blockNum uint32,
) er.R {
	keep := make([]balanceInfo, 0, len(balance.balanceInfo))
	mostRecentBal := balanceInfo{
		balance:  0,
		blockNum: 0,
	}
	for _, bal := range balance.balanceInfo {
		if bal.blockNum > mostRecentBal.blockNum {
			// Track the most recent balance because this is the one we will base our new balance on
			mostRecentBal = bal
		}
		if bal.blockNum >= blockNum {
			// Any balance entry which is *higher* than blockNum gets deleted (rollback)
		} else if epochNum(bal.blockNum) == epochNum(blockNum) {
			// Same epoch, so we replace
		} else if epochNum(blockNum)-epochNum(bal.blockNum) > 1 {
			// More than 1 epoch old, so we prune
		} else {
			keep = append(keep, bal)
		}
	}
	newEntry := balanceInfo{
		balance:  0,
		blockNum: blockNum,
	}

	nb := int64(mostRecentBal.balance) + change
	if nb < 0 {
		// TODO(cjd): Params should be a global so as not to pollute all of the code with passing them around.
		return er.Errorf("Impossible to apply balance change to [%s] at height [%d] "+
			"old balance is [%d @ %d] and change is [%d] which makes negative result [%d]",
			txscript.PkScriptToAddress(balance.addressScript, &chaincfg.PktMainNetParams), blockNum,
			mostRecentBal.balance, mostRecentBal.blockNum, change, nb)
	}
	newEntry.balance = uint64(nb)

	addr := txscript.PkScriptToAddress(balance.addressScript, &chaincfg.PktMainNetParams)
	log.Debugf("Address [%s] changed by [%d] ([%d] -> [%d]) in block [%d]",
		addr.EncodeAddress(), change/(1<<30), mostRecentBal.balance/(1<<30), newEntry.balance/(1<<30), blockNum)

	keep = append(keep, newEntry)
	balance.balanceInfo = keep

	return nil
}
