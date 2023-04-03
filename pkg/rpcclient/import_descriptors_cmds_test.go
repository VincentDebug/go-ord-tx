package rpcclient

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
	"log"
	"testing"
)

func TestImportDescriptorsCmds(t *testing.T) {
	connCfg := &rpcclient.ConnConfig{
		Host:         "localhost:8336",
		User:         "yourrpcuser",
		Pass:         "yourrpcpass",
		HTTPPostMode: true,
		DisableTLS:   true,
	}
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Shutdown()

	net := &chaincfg.SigNetParams

	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		log.Fatal(err)
	}

	privateKeyWIF, err := btcutil.NewWIF(privateKey, net, true)
	if err != nil {
		log.Fatal(err)
	}
	descriptorInfo, err := client.GetDescriptorInfo(fmt.Sprintf("rawtr(%s)", privateKeyWIF))
	if err != nil {
		log.Fatal(err)
	}

	descriptors := []Descriptor{
		{

			Desc: *btcjson.String(fmt.Sprintf("rawtr(%s)#%s", privateKeyWIF, descriptorInfo.Checksum)),
			Timestamp: btcjson.TimestampOrNow{
				Value: "now",
			},
			Active:    btcjson.Bool(false),
			Range:     nil,
			NextIndex: nil,
			Internal:  btcjson.Bool(false),
			Label:     btcjson.String("test label"),
		},
	}

	results, err := ImportDescriptors(client, descriptors)
	if err != nil {
		log.Fatal(err)
	}
	if results == nil {
		log.Fatalf("import failed, nil result")
	}
	for _, result := range *results {
		if !result.Success {
			log.Fatal(errors.New("import failed"))
		}
	}
	log.Printf("Import descriptors success.")
}
