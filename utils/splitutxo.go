package utils

import (
	"fmt"
	"net/http"
	"os"
	"time"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/inc-backend/go-incognito/publish/transformer"
	"github.com/inc-backend/go-incognito/src/httpclient"
	"github.com/inc-backend/go-incognito/src/privatekey"
	"github.com/inc-backend/go-incognito/src/services"
)

const (
	MaxLoopTime = 100
	PRVIDStr    = "0000000000000000000000000000000000000000000000000000000000000004"
)

func SplitUTXOs(endpointUri string, protocol string, privateKey string, paymentAddress string, minNumUTXOs int) error {
	publicIncognito := go_incognito.NewPublicIncognito(
		&http.Client{},
		endpointUri,
		os.Getenv("INCOGNITO_COINSERVICE_URL"),
		2,
	)
	blockInfo := go_incognito.NewBlockInfo(publicIncognito)
	wallet := go_incognito.NewWallet(publicIncognito, blockInfo)

	rpcClient := httpclient.NewHttpClient(endpointUri, os.Getenv("INCOGNITO_COINSERVICE_URL"), protocol, endpointUri, 0)
	senderKeySet, _, err := privatekey.GetKeySetFromPrivateKeyParams(privateKey)
	if err != nil {
		return err
	}

	cntLoop := 0

	for {
		plainCoinsInput, _, err := services.GetListUTXO(rpcClient, PRVIDStr, senderKeySet, 2, true)
		if err != nil {
			return err
		}
		if len(plainCoinsInput) >= minNumUTXOs {
			fmt.Printf("Split UTXOs succeed. Number of UTXOs: %v\n", len(plainCoinsInput))
			break
		}
		if len(plainCoinsInput) == 0 {
			return fmt.Errorf("Could not get any UTXO from this account")
		}

		sumValue := uint64(0)
		for _, coin := range plainCoinsInput {
			sumValue += coin.GetValue()
		}
		avgValue := sumValue / uint64(len(plainCoinsInput))

		fmt.Printf("Number of UTXOs: %v, Avg. Value: %v\n", len(plainCoinsInput), avgValue)
		if avgValue < 10000 {
			return fmt.Errorf("Value of outcoins are too small")
		}

		for idx := 0; idx < len(plainCoinsInput); idx++ {
			result, err := wallet.SendToken(
				privateKey,
				paymentAddress,
				PRVIDStr,
				avgValue/2,
				50,
				"",
			)
			if err != nil {
				continue
			}
			resp, err := publicIncognito.SubmitRawData(result)
			if err != nil {
				continue
			}
			txID, err := transformer.TransformersTxHash(resp)
			if err != nil {
				continue
			}
			fmt.Printf("TxID: %+v\n", txID)
		}

		cntLoop++
		if cntLoop >= MaxLoopTime {
			break
		}

		time.Sleep(1 * time.Minute)
	}
	return nil
}
