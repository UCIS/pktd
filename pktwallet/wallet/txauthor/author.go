// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package txauthor provides transaction creation code for wallets.
package txauthor

import (
	"encoding/binary"
	"io"

	"github.com/pkt-cash/pktd/btcutil/er"

	"github.com/pkt-cash/pktd/btcutil"
	"github.com/pkt-cash/pktd/chaincfg"
	"github.com/pkt-cash/pktd/pktwallet/wallet/txrules"
	"github.com/pkt-cash/pktd/txscript"
	"github.com/pkt-cash/pktd/wire"

	h "github.com/pkt-cash/pktd/pktwallet/internal/helpers"
	"github.com/pkt-cash/pktd/pktwallet/wallet/internal/txsizes"
)

// InputSource provides transaction inputs referencing spendable outputs to
// construct a transaction outputting some target amount.  If the target amount
// can not be satisified, this can be signaled by returning a total amount less
// than the target or by returning a more detailed error implementing
// InputSourceError.
type InputSource func(target btcutil.Amount) (total btcutil.Amount, inputs []*wire.TxIn,
	inputValues []btcutil.Amount, scripts [][]byte, err er.R)

// InputSourceError describes the failure to provide enough input value from
// unspent transaction outputs to meet a target amount.  A typed error is used
// so input sources can provide their own implementations describing the reason
// for the error, for example, due to spendable policies or locked coins rather
// than the wallet not having enough available input value.
var InputSourceError = er.NewErrorType("txauthor.InputSourceError")

// InsufficientFundsError is the default implementation of InputSourceError.
var InsufficientFundsError = InputSourceError.CodeWithDetail("InsufficientFundsError",
	"insufficient funds available to construct transaction")

// AuthoredTx holds the state of a newly-created transaction and the change
// output (if one was added).
type AuthoredTx struct {
	Tx              *wire.MsgTx
	PrevScripts     [][]byte
	PrevInputValues []btcutil.Amount
	TotalInput      btcutil.Amount
	ChangeIndex     int // negative if no change
}

// ChangeSource provides P2PKH change output scripts for transaction creation.
type ChangeSource func() ([]byte, er.R)

func write32(w io.Writer, x uint32) er.R {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], x)
	_, err := w.Write(b[:])
	return er.E(err)
}

func write64(w io.Writer, x uint64) er.R {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], x)
	_, err := w.Write(b[:])
	return er.E(err)
}

func (tx *AuthoredTx) ElectrumSerialize(w io.Writer) er.R {
	// magic
	if _, err := w.Write([]byte("\x00EPTF\xff\x00")); err != nil {
		return er.E(err)
	}

	// tx version
	if err := write32(w, uint32(tx.Tx.Version)); err != nil {
		return err
	}

	// segwit (always yes)
	if _, err := w.Write([]byte("\x00\x01")); err != nil {
		return er.E(err)
	}

	// input count
	if err := wire.WriteVarInt(w, 0, uint64(len(tx.Tx.TxIn))); err != nil {
		return err
	}

	for i, ti := range tx.Tx.TxIn {
		if _, err := w.Write(ti.PreviousOutPoint.Hash[:]); err != nil {
			return er.E(err)
		}
		if err := write32(w, uint32(ti.PreviousOutPoint.Index)); err != nil {
			return err
		}

		// 26ff0023fd<scr>
		scr := make([]byte, len(tx.PrevScripts[i])+1)
		scr[0] = txscript.OP_PUBKEYHASH
		copy(scr[1:], tx.PrevScripts[i])

		scr, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_INVALIDOPCODE).AddOp(txscript.OP_0).AddData(scr).Script()
		if err != nil {
			return err
		}
		if err := wire.WriteVarBytes(w, 0, scr); err != nil {
			return err
		}

		if err := write32(w, ti.Sequence); err != nil {
			return err
		}
	}

	// output count
	if err := wire.WriteVarInt(w, 0, uint64(len(tx.Tx.TxOut))); err != nil {
		return err
	}

	for _, to := range tx.Tx.TxOut {
		if err := wire.WriteTxOut(w, 0, 0, to); err != nil {
			return err
		}
	}

	for _, amt := range tx.PrevInputValues {
		// trick segwit length which informs electrum that it's special data
		if err := wire.WriteVarInt(w, 0, uint64(0xffffffff)); err != nil {
			return err
		}
		// hint for electrum about the amount of the input
		if err := write64(w, uint64(amt)); err != nil {
			return err
		}
		// 2 byte version, 1 byte actual segwit length (zero)
		if _, err := w.Write([]byte("\x00\x00\x00")); err != nil {
			return er.E(err)
		}
	}

	if err := write32(w, tx.Tx.LockTime); err != nil {
		return err
	}

	return nil
}

// NewUnsignedTransaction creates an unsigned transaction paying to one or more
// non-change outputs.  An appropriate transaction fee is included based on the
// transaction size.
//
// Transaction inputs are chosen from repeated calls to fetchInputs with
// increasing targets amounts.
//
// If any remaining output value can be returned to the wallet via a change
// output without violating mempool dust rules, a P2WPKH change output is
// appended to the transaction outputs.  Since the change output may not be
// necessary, fetchChange is called zero or one times to generate this script.
// This function must return a P2WPKH script or smaller, otherwise fee estimation
// will be incorrect.
//
// If successful, the transaction, total input value spent, and all previous
// output scripts are returned.  If the input source was unable to provide
// enough input value to pay for every output any any necessary fees, an
// InputSourceError is returned.
//
// BUGS: Fee estimation may be off when redeeming non-compressed P2PKH outputs.
func NewUnsignedTransaction(outputs []*wire.TxOut, relayFeePerKb btcutil.Amount,
	fetchInputs InputSource, fetchChange ChangeSource) (*AuthoredTx, er.R) {

	targetAmount := h.SumOutputValues(outputs)
	estimatedSize := txsizes.EstimateVirtualSize(0, 1, 0, outputs, true)
	targetFee := txrules.FeeForSerializeSize(relayFeePerKb, estimatedSize)

	for {
		inputAmount, inputs, inputValues, scripts, err := fetchInputs(targetAmount + targetFee)
		if err != nil {
			return nil, err
		}
		if inputAmount < targetAmount+targetFee {
			return nil, InsufficientFundsError.Default()
		}

		// We count the types of inputs, which we'll use to estimate
		// the vsize of the transaction.
		var nested, p2wpkh, p2pkh int
		for _, pkScript := range scripts {
			switch {
			// If this is a p2sh output, we assume this is a
			// nested P2WKH.
			case txscript.IsPayToScriptHash(pkScript):
				nested++
			case txscript.IsPayToWitnessPubKeyHash(pkScript):
				p2wpkh++
			default:
				p2pkh++
			}
		}

		maxSignedSize := txsizes.EstimateVirtualSize(p2pkh, p2wpkh,
			nested, outputs, true)
		maxRequiredFee := txrules.FeeForSerializeSize(relayFeePerKb, maxSignedSize)
		remainingAmount := inputAmount - targetAmount
		if remainingAmount < maxRequiredFee {
			targetFee = maxRequiredFee
			continue
		}

		unsignedTransaction := &wire.MsgTx{
			Version:  wire.TxVersion,
			TxIn:     inputs,
			TxOut:    outputs,
			LockTime: 0,
		}
		changeIndex := -1
		changeAmount := inputAmount - targetAmount - maxRequiredFee
		if changeAmount != 0 && !txrules.IsDustAmount(changeAmount,
			txsizes.P2WPKHPkScriptSize, relayFeePerKb) {
			changeScript, err := fetchChange()
			if err != nil {
				return nil, err
			}
			// if len(changeScript) > txsizes.P2WPKHPkScriptSize {
			// 	return nil, er.New("fee estimation requires change " +
			// 		"scripts no larger than P2WPKH output scripts")
			// }
			change := wire.NewTxOut(int64(changeAmount), changeScript)
			l := len(outputs)
			unsignedTransaction.TxOut = append(outputs[:l:l], change)
			changeIndex = l
		}

		return &AuthoredTx{
			Tx:              unsignedTransaction,
			PrevScripts:     scripts,
			PrevInputValues: inputValues,
			TotalInput:      inputAmount,
			ChangeIndex:     changeIndex,
		}, nil
	}
}

// RandomizeOutputPosition randomizes the position of a transaction's output by
// swapping it with a random output.  The new index is returned.  This should be
// done before signing.
func RandomizeOutputPosition(outputs []*wire.TxOut, index int) int {
	r := cprng.Int31n(int32(len(outputs)))
	outputs[r], outputs[index] = outputs[index], outputs[r]
	return int(r)
}

// RandomizeChangePosition randomizes the position of an authored transaction's
// change output.  This should be done before signing.
func (tx *AuthoredTx) RandomizeChangePosition() {
	tx.ChangeIndex = RandomizeOutputPosition(tx.Tx.TxOut, tx.ChangeIndex)
}

// SecretsSource provides private keys and redeem scripts necessary for
// constructing transaction input signatures.  Secrets are looked up by the
// corresponding Address for the previous output script.  Addresses for lookup
// are created using the source's blockchain parameters and means a single
// SecretsSource can only manage secrets for a single chain.
//
// TODO: Rewrite this interface to look up private keys and redeem scripts for
// pubkeys, pubkey hashes, script hashes, etc. as separate interface methods.
// This would remove the ChainParams requirement of the interface and could
// avoid unnecessary conversions from previous output scripts to Addresses.
// This can not be done without modifications to the txscript package.
type SecretsSource interface {
	txscript.KeyDB
	txscript.ScriptDB
	ChainParams() *chaincfg.Params
}

// AddAllInputScripts modifies transaction a transaction by adding inputs
// scripts for each input.  Previous output scripts being redeemed by each input
// are passed in prevPkScripts and the slice length must match the number of
// inputs.  Private keys and redeem scripts are looked up using a SecretsSource
// based on the previous output script.
func AddAllInputScripts(tx *wire.MsgTx, prevPkScripts [][]byte, inputValues []btcutil.Amount,
	secrets SecretsSource) er.R {

	inputs := tx.TxIn
	hashCache := txscript.NewTxSigHashes(tx)
	chainParams := secrets.ChainParams()

	if len(inputs) != len(prevPkScripts) {
		return er.New("tx.TxIn and prevPkScripts slices must " +
			"have equal length")
	}

	for i := range inputs {
		pkScript := prevPkScripts[i]

		switch {
		// If this is a p2sh output, who's script hash pre-image is a
		// witness program, then we'll need to use a modified signing
		// function which generates both the sigScript, and the witness
		// script.
		case txscript.IsPayToScriptHash(pkScript):
			err := spendNestedWitnessPubKeyHash(inputs[i], pkScript,
				int64(inputValues[i]), chainParams, secrets,
				tx, hashCache, i)
			if err != nil {
				return err
			}
		case txscript.IsPayToWitnessPubKeyHash(pkScript):
			err := spendWitnessKeyHash(inputs[i], pkScript,
				int64(inputValues[i]), chainParams, secrets,
				tx, hashCache, i)
			if err != nil {
				return err
			}
		default:
			sigScript := inputs[i].SignatureScript
			script, err := txscript.SignTxOutput(chainParams, tx, i,
				pkScript, txscript.SigHashAll, secrets, secrets,
				sigScript)
			if err != nil {
				return err
			}
			inputs[i].SignatureScript = script
		}
	}

	return nil
}

// spendWitnessKeyHash generates, and sets a valid witness for spending the
// passed pkScript with the specified input amount. The input amount *must*
// correspond to the output value of the previous pkScript, or else verification
// will fail since the new sighash digest algorithm defined in BIP0143 includes
// the input value in the sighash.
func spendWitnessKeyHash(txIn *wire.TxIn, pkScript []byte,
	inputValue int64, chainParams *chaincfg.Params, secrets SecretsSource,
	tx *wire.MsgTx, hashCache *txscript.TxSigHashes, idx int) er.R {

	// First obtain the key pair associated with this p2wkh address.
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript,
		chainParams)
	if err != nil {
		return err
	}
	privKey, compressed, err := secrets.GetKey(addrs[0])
	if err != nil {
		return err
	}
	pubKey := privKey.PubKey()

	// Once we have the key pair, generate a p2wkh address type, respecting
	// the compression type of the generated key.
	var pubKeyHash []byte
	if compressed {
		pubKeyHash = btcutil.Hash160(pubKey.SerializeCompressed())
	} else {
		pubKeyHash = btcutil.Hash160(pubKey.SerializeUncompressed())
	}
	p2wkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, chainParams)
	if err != nil {
		return err
	}

	// With the concrete address type, we can now generate the
	// corresponding witness program to be used to generate a valid witness
	// which will allow us to spend this output.
	witnessProgram, err := txscript.PayToAddrScript(p2wkhAddr)
	if err != nil {
		return err
	}
	witnessScript, err := txscript.WitnessSignature(tx, hashCache, idx,
		inputValue, witnessProgram, txscript.SigHashAll, privKey, true)
	if err != nil {
		return err
	}

	txIn.Witness = witnessScript

	return nil
}

// spendNestedWitnessPubKey generates both a sigScript, and valid witness for
// spending the passed pkScript with the specified input amount. The generated
// sigScript is the version 0 p2wkh witness program corresponding to the queried
// key. The witness stack is identical to that of one which spends a regular
// p2wkh output. The input amount *must* correspond to the output value of the
// previous pkScript, or else verification will fail since the new sighash
// digest algorithm defined in BIP0143 includes the input value in the sighash.
func spendNestedWitnessPubKeyHash(txIn *wire.TxIn, pkScript []byte,
	inputValue int64, chainParams *chaincfg.Params, secrets SecretsSource,
	tx *wire.MsgTx, hashCache *txscript.TxSigHashes, idx int) er.R {

	// First we need to obtain the key pair related to this p2sh output.
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript,
		chainParams)
	if err != nil {
		return err
	}
	privKey, compressed, err := secrets.GetKey(addrs[0])
	if err != nil {
		return err
	}
	pubKey := privKey.PubKey()

	var pubKeyHash []byte
	if compressed {
		pubKeyHash = btcutil.Hash160(pubKey.SerializeCompressed())
	} else {
		pubKeyHash = btcutil.Hash160(pubKey.SerializeUncompressed())
	}

	// Next, we'll generate a valid sigScript that'll allow us to spend
	// the p2sh output. The sigScript will contain only a single push of
	// the p2wkh witness program corresponding to the matching public key
	// of this address.
	p2wkhAddr, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, chainParams)
	if err != nil {
		return err
	}
	witnessProgram, err := txscript.PayToAddrScript(p2wkhAddr)
	if err != nil {
		return err
	}
	bldr := txscript.NewScriptBuilder()
	bldr.AddData(witnessProgram)
	sigScript, err := bldr.Script()
	if err != nil {
		return err
	}
	txIn.SignatureScript = sigScript

	// With the sigScript in place, we'll next generate the proper witness
	// that'll allow us to spend the p2wkh output.
	witnessScript, err := txscript.WitnessSignature(tx, hashCache, idx,
		inputValue, witnessProgram, txscript.SigHashAll, privKey, compressed)
	if err != nil {
		return err
	}

	txIn.Witness = witnessScript

	return nil
}

// AddAllInputScripts modifies an authored transaction by adding inputs scripts
// for each input of an authored transaction.  Private keys and redeem scripts
// are looked up using a SecretsSource based on the previous output script.
func (tx *AuthoredTx) AddAllInputScripts(secrets SecretsSource) er.R {
	return AddAllInputScripts(tx.Tx, tx.PrevScripts, tx.PrevInputValues, secrets)
}