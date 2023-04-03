package ord

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	extRpcClient "go-ord-tx/pkg/rpcclient"
	"log"
)

type InscriptionData struct {
	ContentType string
	Body        []byte
	Destination string
}

type InscriptionRequest struct {
	CommitTxOutPointList []*wire.OutPoint
	CommitFeeRate        int64
	FeeRate              int64
	DataList             []InscriptionData
	SingleRevealTxOnly   bool // Currently, the official Ordinal parser can only parse a single NFT per transaction. When the official Ordinal parser supports parsing multiple NFTs in the future, we can consider using a single reveal transaction.
}

type inscriptionTxCtxData struct {
	privateKey              *btcec.PrivateKey
	inscriptionScript       []byte
	commitTxAddressPkScript []byte
	controlBlockWitness     []byte
	recoveryPrivateKeyWIF   string
	revealTxPrevOutput      *wire.TxOut
}

type InscriptionTool struct {
	net           *chaincfg.Params
	client        *rpcclient.Client
	txCtxDataList []*inscriptionTxCtxData
	revealTx      []*wire.MsgTx
	commitTx      *wire.MsgTx
}

const (
	defaultSequenceNum    = wire.MaxTxInSequenceNum - 10
	defaultRevealOutValue = int64(500) // 500 sat, ord default 10000

)

func InitInscriptionTool(net *chaincfg.Params, client *rpcclient.Client, request *InscriptionRequest) (*InscriptionTool, error) {
	tool := &InscriptionTool{
		net:           net,
		client:        client,
		txCtxDataList: make([]*inscriptionTxCtxData, len(request.DataList)),
	}

	destinations := make([]string, len(request.DataList))
	for i := 0; i < len(request.DataList); i++ {
		txCtxData, err := createInscriptionTxCtxData(net, request, i)
		if err != nil {
			return nil, err
		}
		tool.txCtxDataList[i] = txCtxData
		destinations[i] = request.DataList[i].Destination

	}
	totalRevealPrevOutput, err := tool.buildEmptyRevealTx(request.SingleRevealTxOnly, destinations, request.FeeRate)
	if err != nil {
		return nil, err
	}
	err = tool.buildCommitTx(request.CommitTxOutPointList, totalRevealPrevOutput, request.CommitFeeRate)
	if err != nil {
		return nil, err
	}
	err = tool.completeRevealTx()
	return tool, err
}

func createInscriptionTxCtxData(net *chaincfg.Params, inscriptionRequest *InscriptionRequest, indexOfRequestDataList int) (*inscriptionTxCtxData, error) {
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}
	inscriptionBuilder := txscript.NewScriptBuilder().
		AddData(schnorr.SerializePubKey(privateKey.PubKey())).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_FALSE).
		AddOp(txscript.OP_IF).
		AddData([]byte("ord")).
		// Two OP_DATA_1 should be OP_0. However, in the following link, it's not set as OP_0:
		// https://github.com/casey/ord/blob/0.5.1/src/inscription.rs#L16
		// Therefore, we use two OP_DATA_1 to maintain consistency with ord.
		AddOp(txscript.OP_DATA_1).
		AddOp(txscript.OP_DATA_1).
		AddData([]byte(inscriptionRequest.DataList[indexOfRequestDataList].ContentType)).
		AddOp(txscript.OP_0)
	maxChunkSize := 520
	bodySize := len(inscriptionRequest.DataList[indexOfRequestDataList].Body)
	for i := 0; i < bodySize; i += maxChunkSize {
		end := i + maxChunkSize
		if end > bodySize {
			end = bodySize
		}
		inscriptionBuilder.AddData(inscriptionRequest.DataList[indexOfRequestDataList].Body[i:end])
	}
	inscriptionScript, err := inscriptionBuilder.AddOp(txscript.OP_ENDIF).Script()

	if err != nil {
		return nil, err
	}

	proof := &txscript.TapscriptProof{
		TapLeaf:  txscript.NewBaseTapLeaf(schnorr.SerializePubKey(privateKey.PubKey())),
		RootNode: txscript.NewBaseTapLeaf(inscriptionScript),
	}

	controlBlock := proof.ToControlBlock(privateKey.PubKey())
	controlBlockWitness, err := controlBlock.ToBytes()
	if err != nil {
		return nil, err
	}

	tapHash := proof.RootNode.TapHash()
	commitTxAddress, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(txscript.ComputeTaprootOutputKey(privateKey.PubKey(), tapHash[:])), net)
	if err != nil {
		return nil, err
	}
	commitTxAddressPkScript, err := txscript.PayToAddrScript(commitTxAddress)
	if err != nil {
		return nil, err
	}

	recoveryPrivateKeyWIF, err := btcutil.NewWIF(txscript.TweakTaprootPrivKey(*privateKey, tapHash[:]), net, true)
	if err != nil {
		return nil, err
	}

	return &inscriptionTxCtxData{
		privateKey:              privateKey,
		inscriptionScript:       inscriptionScript,
		commitTxAddressPkScript: commitTxAddressPkScript,
		controlBlockWitness:     controlBlockWitness,
		recoveryPrivateKeyWIF:   recoveryPrivateKeyWIF.String(),
	}, nil
}

func (tool *InscriptionTool) buildEmptyRevealTx(singleRevealTxOnly bool, destination []string, feeRate int64) (int64, error) {
	revealOutValue := defaultRevealOutValue
	var revealTx []*wire.MsgTx
	totalPrevOutput := int64(0)
	total := len(tool.txCtxDataList)
	addTxInTxOutIntoRevealTx := func(tx *wire.MsgTx, index int) error {
		in := wire.NewTxIn(&wire.OutPoint{Index: uint32(index)}, nil, nil)
		in.Sequence = defaultSequenceNum
		tx.AddTxIn(in)
		receiver, err := btcutil.DecodeAddress(destination[index], tool.net)
		if err != nil {
			return err
		}
		scriptPubKey, err := txscript.PayToAddrScript(receiver)
		if err != nil {
			return err
		}
		out := wire.NewTxOut(revealOutValue, scriptPubKey)
		tx.AddTxOut(out)
		return nil
	}
	if singleRevealTxOnly {
		revealTx = make([]*wire.MsgTx, 1)
		tx := wire.NewMsgTx(wire.TxVersion)
		for i := 0; i < total; i++ {
			err := addTxInTxOutIntoRevealTx(tx, i)
			if err != nil {
				return 0, err
			}
		}
		eachRevealBaseTxFee := int64(tx.SerializeSize()) * feeRate / int64(total)
		prevOutput := (revealOutValue + eachRevealBaseTxFee) * int64(total)
		{
			emptySignature := make([]byte, 64)
			emptyControlBlockWitness := make([]byte, 33)
			for i := 0; i < total; i++ {
				fee := (int64(wire.TxWitness{emptySignature, tool.txCtxDataList[i].inscriptionScript, emptyControlBlockWitness}.SerializeSize()+2+3) / 4) * feeRate
				tool.txCtxDataList[i].revealTxPrevOutput = &wire.TxOut{
					PkScript: tool.txCtxDataList[i].commitTxAddressPkScript,
					Value:    revealOutValue + eachRevealBaseTxFee + fee,
				}
				prevOutput += fee
			}
		}
		totalPrevOutput = prevOutput
		revealTx[0] = tx
	} else {
		revealTx = make([]*wire.MsgTx, total)
		for i := 0; i < total; i++ {
			tx := wire.NewMsgTx(wire.TxVersion)
			err := addTxInTxOutIntoRevealTx(tx, i)
			if err != nil {
				return 0, err
			}
			prevOutput := revealOutValue + int64(tx.SerializeSize())*feeRate
			{
				emptySignature := make([]byte, 64)
				emptyControlBlockWitness := make([]byte, 33)
				fee := (int64(wire.TxWitness{emptySignature, tool.txCtxDataList[i].inscriptionScript, emptyControlBlockWitness}.SerializeSize()+2+3) / 4) * feeRate
				prevOutput += fee
				tool.txCtxDataList[i].revealTxPrevOutput = &wire.TxOut{
					PkScript: tool.txCtxDataList[i].commitTxAddressPkScript,
					Value:    prevOutput,
				}
			}
			totalPrevOutput += prevOutput
			revealTx[i] = tx
		}
	}
	tool.revealTx = revealTx
	return totalPrevOutput, nil
}

func (tool *InscriptionTool) buildCommitTx(commitTxOutPointList []*wire.OutPoint, totalRevealPrevOutput, commitFeeRate int64) error {
	totalSenderAmount := btcutil.Amount(0)
	tx := wire.NewMsgTx(wire.TxVersion)
	var changePkScript *[]byte
	for i := range commitTxOutPointList {
		outPutTx, err := tool.client.GetRawTransactionVerbose(&commitTxOutPointList[i].Hash)
		if err != nil {
			return err
		}
		if int(commitTxOutPointList[i].Index) >= len(outPutTx.Vout) {
			return errors.New("err out point")
		}
		prevOutput := outPutTx.Vout[commitTxOutPointList[i].Index]
		senderPkScript, err := hex.DecodeString(prevOutput.ScriptPubKey.Hex)
		if err != nil {
			return err
		}
		if changePkScript == nil { // first sender as change address
			changePkScript = &senderPkScript
		}
		in := wire.NewTxIn(commitTxOutPointList[i], nil, nil)
		in.Sequence = defaultSequenceNum
		tx.AddTxIn(in)

		amount, err := btcutil.NewAmount(prevOutput.Value)
		if err != nil {
			return err
		}
		totalSenderAmount += amount
	}
	for i := range tool.txCtxDataList {
		tx.AddTxOut(tool.txCtxDataList[i].revealTxPrevOutput)
	}

	tx.AddTxOut(wire.NewTxOut(0, *changePkScript))
	fee := btcutil.Amount(mempool.GetTxVirtualSize(btcutil.NewTx(tx))) * btcutil.Amount(commitFeeRate)
	changeAmount := totalSenderAmount - btcutil.Amount(totalRevealPrevOutput) - fee
	if changeAmount > 0 {
		tx.TxOut[len(tx.TxOut)-1].Value = int64(changeAmount)
	} else {
		tx.TxOut = tx.TxOut[:len(tx.TxOut)-1]
		if changeAmount < 0 {
			feeWithoutChange := btcutil.Amount(mempool.GetTxVirtualSize(btcutil.NewTx(tx))) * btcutil.Amount(commitFeeRate)
			if totalSenderAmount-btcutil.Amount(totalRevealPrevOutput)-feeWithoutChange < 0 {
				return errors.New("insufficient balance")
			}
		}
	}
	tool.commitTx = tx
	return nil
}

func (tool *InscriptionTool) completeRevealTx() error {
	revealTxPrevOutputFetcher := txscript.NewMultiPrevOutFetcher(nil)
	for i := range tool.txCtxDataList {
		revealTxPrevOutputFetcher.AddPrevOut(wire.OutPoint{
			Hash:  tool.commitTx.TxHash(),
			Index: uint32(i),
		}, tool.txCtxDataList[i].revealTxPrevOutput)
		if len(tool.revealTx) == 1 {
			tool.revealTx[0].TxIn[i].PreviousOutPoint.Hash = tool.commitTx.TxHash()
		} else {
			tool.revealTx[i].TxIn[0].PreviousOutPoint.Hash = tool.commitTx.TxHash()
		}
	}
	witnessList := make([]wire.TxWitness, len(tool.txCtxDataList))
	for i := range tool.txCtxDataList {
		revealTx := tool.revealTx[0]
		idx := i
		if len(tool.revealTx) != 1 {
			revealTx = tool.revealTx[i]
			idx = 0
		}
		witnessArray, err := txscript.CalcTapscriptSignaturehash(txscript.NewTxSigHashes(revealTx, revealTxPrevOutputFetcher), txscript.SigHashDefault, revealTx, idx, revealTxPrevOutputFetcher, txscript.NewBaseTapLeaf(tool.txCtxDataList[i].inscriptionScript))
		if err != nil {
			return err
		}
		signature, err := schnorr.Sign(tool.txCtxDataList[i].privateKey, witnessArray)
		if err != nil {
			return err
		}
		witnessList[i] = wire.TxWitness{signature.Serialize(), tool.txCtxDataList[i].inscriptionScript, tool.txCtxDataList[i].controlBlockWitness}
	}
	for i := range witnessList {
		if len(tool.revealTx) == 1 {
			tool.revealTx[0].TxIn[i].Witness = witnessList[i]
		} else {
			tool.revealTx[i].TxIn[0].Witness = witnessList[i]
		}
	}
	return nil
}

func (tool *InscriptionTool) backupRecoveryKey() error {
	descriptors := make([]extRpcClient.Descriptor, len(tool.txCtxDataList))
	for i := range tool.txCtxDataList {
		descriptorInfo, err := tool.client.GetDescriptorInfo(fmt.Sprintf("rawtr(%s)", tool.txCtxDataList[i].recoveryPrivateKeyWIF))
		if err != nil {
			return err
		}
		descriptors[i] = extRpcClient.Descriptor{
			Desc: *btcjson.String(fmt.Sprintf("rawtr(%s)#%s", tool.txCtxDataList[i].recoveryPrivateKeyWIF, descriptorInfo.Checksum)),
			Timestamp: btcjson.TimestampOrNow{
				Value: "now",
			},
			Active:    btcjson.Bool(false),
			Range:     nil,
			NextIndex: nil,
			Internal:  btcjson.Bool(false),
			Label:     btcjson.String("commit tx recovery key"),
		}
	}
	results, err := extRpcClient.ImportDescriptors(tool.client, descriptors)
	if err != nil {
		return err
	}
	if results == nil {
		return errors.New("commit tx recovery key import failed, nil result")
	}
	for _, result := range *results {
		if !result.Success {
			return errors.New("commit tx recovery key import failed")
		}
	}
	return nil
}

func (tool *InscriptionTool) Send() (commitTxHash *chainhash.Hash, revealTxHash []*chainhash.Hash, err error) {
	err = tool.backupRecoveryKey()
	if err != nil {
		return nil, nil, err
	}
	commitSignTransaction, isSignComplete, err := tool.client.SignRawTransactionWithWallet(tool.commitTx)
	if err != nil {
		log.Printf("sign commit tx error, %v", err)
		return nil, nil, err
	}
	if !isSignComplete {
		return nil, nil, errors.New("sign commit tx error")
	}
	commitTxHash, err = tool.client.SendRawTransaction(commitSignTransaction, false)
	if err != nil {
		log.Printf("send commit tx error, %v", err)
		return nil, nil, err
	}
	revealTxHash = make([]*chainhash.Hash, len(tool.revealTx))
	for i := range tool.revealTx {
		_revealTxHash, err := tool.client.SendRawTransaction(tool.revealTx[i], false)
		if err != nil {
			log.Printf("send reveal tx error, %d, %v", i, err)
			return commitTxHash, revealTxHash, err
		}
		revealTxHash[i] = _revealTxHash
	}
	return commitTxHash, revealTxHash, nil
}
