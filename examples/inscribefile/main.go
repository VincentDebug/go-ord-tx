package main

import (
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/vincentdebug/go-ord-tx/ord"
	"github.com/vincentdebug/go-ord-tx/pkg/btcapi/mempool"
	"log"
	"net/http"
	"os"
)

func main() {
	netParams := &chaincfg.SigNetParams
	btcApiClient := mempool.NewClient(netParams)

	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Error getting current working directory, %v", err)
	}
	filePath := fmt.Sprintf("%s/examples/inscribefile/142KB.jpeg", workingDir)
	// if file size too max will return sendrawtransaction RPC error: {"code":-26,"message":"tx-size"}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading file %v", err)
	}

	contentType := http.DetectContentType(fileContent)
	log.Printf("file contentType %s", contentType)

	utxoPrivateKeyHex := "your private key"
	destination := "tb1p3m6qfu0mzkxsmaue0hwekrxm2nxfjjrmv4dvy94gxs8c3s7zns6qcgf8ef"

	commitTxOutPointList := make([]*wire.OutPoint, 0)
	commitTxPrivateKeyList := make([]*btcec.PrivateKey, 0)

	{
		utxoPrivateKeyBytes, err := hex.DecodeString(utxoPrivateKeyHex)
		if err != nil {
			log.Fatal(err)
		}
		utxoPrivateKey, _ := btcec.PrivKeyFromBytes(utxoPrivateKeyBytes)

		utxoTaprootAddress, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(txscript.ComputeTaprootKeyNoScript(utxoPrivateKey.PubKey())), netParams)
		if err != nil {
			log.Fatal(err)
		}

		unspentList, err := btcApiClient.ListUnspent(utxoTaprootAddress)

		if err != nil {
			log.Fatalf("list unspent err %v", err)
		}

		for i := range unspentList {
			commitTxOutPointList = append(commitTxOutPointList, unspentList[i].Outpoint)
			commitTxPrivateKeyList = append(commitTxPrivateKeyList, utxoPrivateKey)
		}
	}

	request := ord.InscriptionRequest{
		CommitTxOutPointList:   commitTxOutPointList,
		CommitTxPrivateKeyList: commitTxPrivateKeyList,
		CommitFeeRate:          2,
		FeeRate:                1,
		DataList: []ord.InscriptionData{
			{
				ContentType: contentType,
				Body:        fileContent,
				Destination: destination,
			},
		},
		SingleRevealTxOnly: false,
	}

	tool, err := ord.NewInscriptionToolWithBtcApiClient(netParams, btcApiClient, &request)
	if err != nil {
		log.Fatalf("Failed to create inscription tool: %v", err)
	}
	commitTxHash, revealTxHashList, inscriptions, fees, err := tool.Inscribe()
	if err != nil {
		log.Fatalf("send tx errr, %v", err)
	}
	log.Println("commitTxHash, " + commitTxHash.String())
	for i := range revealTxHashList {
		log.Println("revealTxHash, " + revealTxHashList[i].String())
	}
	for i := range inscriptions {
		log.Println("inscription, " + inscriptions[i])
	}
	log.Println("fees: ", fees)
}
