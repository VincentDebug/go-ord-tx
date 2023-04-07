package main

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"go-ord-tx/internal/ord"
	"log"
)

func main() {
	netParams := &chaincfg.SigNetParams
	connCfg := &rpcclient.ConnConfig{
		Host:         "yourhost:8332",
		User:         "yourrpcuser",
		Pass:         "yourrpcpass",
		HTTPPostMode: true,
		DisableTLS:   false,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatalf("Failed to create RPC client: %v", err)
	}
	defer client.Shutdown()

	utxoPrivateKeyHex := "your private key"
	utxoPrivateKeyBytes, err := hex.DecodeString(utxoPrivateKeyHex)
	if err != nil {
		log.Fatal(err)
	}
	utxoPrivateKey, _ := btcec.PrivKeyFromBytes(utxoPrivateKeyBytes)

	utxoTaprootAddress, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(txscript.ComputeTaprootKeyNoScript(utxoPrivateKey.PubKey())), netParams)
	if err != nil {
		log.Fatal(err)
	}

	// you can get from `client.ListUnspent()`
	//utxoAddress := "tb1p8lh4np5824u48ppawq3numsm7rss0de4kkxry0z70dcfwwwn2fcspyyhc7"

	info, err := client.GetBlockChainInfo()
	if err != nil {
		log.Fatalf("blockchaininfo err %v", err)
	}
	log.Printf("%v", info)
	unspentList, err := client.ListUnspentMinMaxAddresses(1, 9999999, []btcutil.Address{utxoTaprootAddress})

	if err != nil {
		log.Fatalf("list unspent err %v", err)
	}

	commitTxOutPointList := make([]*wire.OutPoint, 0)
	commitTxPrivateKeyList := make([]*btcec.PrivateKey, 0)
	for i := range unspentList {
		inTxid, err := chainhash.NewHashFromStr(unspentList[i].TxID)
		if err != nil {
			log.Fatalf("decode in hash err %v", err)
		}
		commitTxOutPointList = append(commitTxOutPointList, wire.NewOutPoint(inTxid, unspentList[i].Vout))
		commitTxPrivateKeyList = append(commitTxPrivateKeyList, utxoPrivateKey)
	}

	dataList := make([]ord.InscriptionData, 0)

	dataList = append(dataList, ord.InscriptionData{
		ContentType: "text/plain;charset=utf-8",
		Body:        []byte("Create with public node"),
		Destination: "tb1p3m6qfu0mzkxsmaue0hwekrxm2nxfjjrmv4dvy94gxs8c3s7zns6qcgf8ef",
	})

	request := ord.InscriptionRequest{
		CommitTxOutPointList:   commitTxOutPointList,
		CommitTxPrivateKeyList: commitTxPrivateKeyList,
		CommitFeeRate:          25,
		FeeRate:                26,
		DataList:               dataList,
		SingleRevealTxOnly:     false,
	}

	tool, err := ord.NewInscriptionTool(netParams, client, &request)
	if err != nil {
		log.Fatalf("Failed to create inscription tool: %v", err)
	}
	// Please avoid backing up your recovery key to a public RPC node using tool.BackupRecoveryKeyToRpcNode(). It is highly recommended to handle the backup and storage of your recovery key by yourself.
	recoveryKeyWIFList := tool.GetRecoveryKeyWIFList()
	for i, recoveryKeyWIF := range recoveryKeyWIFList {
		log.Printf("recoveryKeyWIF %d %s \n", i, recoveryKeyWIF)
	}

	commitTxHex, err := tool.GetCommitTxHex()
	if err != nil {
		log.Fatalf("get commit tx hex err, %v", err)
	}
	log.Printf("commitTxHex %s \n", commitTxHex)
	revealTxHexList, err := tool.GetRevealTxHexList()
	if err != nil {
		log.Fatalf("get reveal tx hex err, %v", err)
	}
	for i, revealTxHex := range revealTxHexList {
		log.Printf("revealTxHex %d %s \n", i, revealTxHex)
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
