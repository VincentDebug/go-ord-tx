package main

import (
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/tyler-smith/go-bip39"
	"log"
)

func main() {
	// random mnemonic 24 words
	entropy, _ := bip39.NewEntropy(256)
	randomMnemonic, _ := bip39.NewMnemonic(entropy)
	fmt.Println(randomMnemonic)

	mnemonic := randomMnemonic           // "your mnemonic"
	netParams := &chaincfg.MainNetParams // for generate address
	rootKey, err := hdkeychain.NewMaster(bip39.NewSeed(mnemonic, ""), netParams)
	if err != nil {
		log.Fatal(err)
	}

	// for sparrow wallet receive addresses a path is start "m/86'/0'/0'/0/0"; change addresses a path is start "m/86'/0'/0'/1/0"
	childKey, _ := rootKey.Derive(hdkeychain.HardenedKeyStart + 86) // 86'
	childKey, _ = childKey.Derive(hdkeychain.HardenedKeyStart + 0)  // 0'
	childKey, _ = childKey.Derive(hdkeychain.HardenedKeyStart + 0)  // 0'
	childKey, _ = childKey.Derive(1)                                // 1
	senderKey, _ := childKey.Derive(0)                              // 0
	senderPrivateKey, _ := senderKey.ECPrivKey()
	privateKeyHex := hex.EncodeToString(senderPrivateKey.Serialize())
	log.Printf("priviate key: %s \n", privateKeyHex)
	taprootAddress, err := btcutil.NewAddressTaproot(schnorr.SerializePubKey(txscript.ComputeTaprootKeyNoScript(senderPrivateKey.PubKey())), netParams)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("taproot address: %s \n", taprootAddress.EncodeAddress())
}
