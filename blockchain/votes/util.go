package votes

import (
	"bytes"
	"encoding/binary"

	"github.com/pkt-cash/pktd/blockchain"
	"github.com/pkt-cash/pktd/btcutil"
	"github.com/pkt-cash/pktd/btcutil/er"
	"github.com/pkt-cash/pktd/pktlog/log"
	"github.com/pkt-cash/pktd/txscript/opcode"
	"github.com/pkt-cash/pktd/txscript/parsescript"
)

const (
	VOTE      byte = 0x00
	CANDIDATE byte = 0x01
)

func getVote(outputScript []byte) *NsVote {
	scr, err := parsescript.ParseScript(outputScript)
	// txscript.ElectionGetVotesForAgainst()
	if err != nil {
		return nil
	}
	if len(scr) < 1 || scr[0].Opcode.Value != opcode.OP_RETURN {
		// Normal script, does not begin with OP_RETURN
		return nil
	}
	if len(scr) < 2 || scr[1].Opcode.Value > opcode.OP_16 {
		// It's an op-return script which contains something other than a push
		return nil
	}
	if len(scr) > 2 {
		// it's an op-return script but it contains additional data after the push
		return nil
	}
	data := scr[1].Data
	if len(data) < 1 || (data[0] != VOTE && data[0] != CANDIDATE) {
		// Not a vote operation
		return nil
	}
	return &NsVote{
		VoterIsWillingCandidate: data[0] == CANDIDATE,
		VoteForPkScript:         data[1:],
	}
}

func encodeVote(v NsVote) ([]byte, []byte) {
	vk := make([]byte, 4+len(v.VoterPkScript))
	binary.BigEndian.PutUint32(vk[:4], v.VoteCastInBlock)
	copy(vk[4:], v.VoterPkScript)
	vv := make([]byte, len(v.VoteForPkScript)+1)
	if v.VoterIsWillingCandidate {
		vv[0] = 1
	} else {
		vv[0] = 0
	}
	copy(vv[1:], v.VoteForPkScript)
	return vk, vv
}

func decodeVote(k, v []byte) (*NsVote, er.R) {
	if len(k) < 4 {
		return nil, er.Errorf("Vote has runt key (length: [%d])", len(k))
	} else if len(v) < 1 {
		return nil, er.Errorf("Vote has runt value (length: [%d])", len(k))
	}
	return &NsVote{
		VoterPkScript:           k[4:],
		VoterIsWillingCandidate: v[0] == 1,
		VoteCastInBlock:         binary.BigEndian.Uint32(k[:4]),
		VoteForPkScript:         v[1:],
	}, nil
}

func parseVotes(block *btcutil.Block, stxo []blockchain.SpentTxOut) ([]NsVote, er.R) {
	stxoIdx := 0
	var blockVotes []NsVote
txns:
	for _, tx := range block.Transactions()[1:] {
		inputs := stxo[stxoIdx : stxoIdx+len(tx.MsgTx().TxIn)]
		stxoIdx += len(tx.MsgTx().TxIn)

		var vote *NsVote
		for _, out := range tx.MsgTx().TxOut {
			v := getVote(out.PkScript)
			if v != nil {
				if vote != nil {
					log.Infof("Ignoring votes in transaction [%s@%d], a transaction can only have one vote",
						tx.Hash(), block.Height())
					continue txns
				}
				vote = v
			}
		}
		if vote == nil {
			// No votes
			continue txns
		}

		// There is no explicit mapping between the stxos and the block transactions, but they
		// are in the same order so we can walk through the stxos as we walk though the inputs
		if len(inputs) != len(tx.MsgTx().TxIn) {
			return nil, er.Errorf("Mismatch in number of spent txouts for txn [%s@%d] "+
				"expect [%d] got [%d]", tx.Hash(), block.Height(), len(tx.MsgTx().TxIn), len(inputs))
		}
		var addr []byte
		for _, inp := range inputs {
			if addr == nil {
				addr = inp.PkScript
			} else if !bytes.Equal(addr, inp.PkScript) {
				log.Infof("Ignoring vote in transaction [%s@%d], only one input address is allowed",
					tx.Hash(), block.Height())
				continue txns
			}
		}

		vote.VoteCastInBlock = uint32(block.Height())
		vote.VoterPkScript = addr

		blockVotes = append(blockVotes, *vote)
	}

	if stxoIdx != len(stxo) {
		return nil, er.Errorf("Transactions in block [%d] have a total of [%d] SpentTxOut but [%d] txins "+
			"this should not happen.", block.Height(), len(stxo), stxoIdx)
	}

	return blockVotes, nil
}
