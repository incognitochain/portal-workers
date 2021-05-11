package utils

import (
	"fmt"
	"net/http"
	"time"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/inc-backend/go-incognito/publish/transformer"
	"github.com/inc-backend/go-incognito/src/httpclient"
	"github.com/inc-backend/go-incognito/src/privatekey"
	"github.com/inc-backend/go-incognito/src/services"
)

func SplitUTXOs(endpointUri string, protocol string, privateKey string, paymentAddress string, minNumUTXOs int) error {
	publicIncognito := go_incognito.NewPublicIncognito(
		&http.Client{},
		endpointUri,
		"",
		2,
	)
	blockInfo := go_incognito.NewBlockInfo(publicIncognito)
	wallet := go_incognito.NewWallet(publicIncognito, blockInfo)

	rpcClient := httpclient.NewHttpClient(endpointUri, "", protocol, endpointUri, 0)
	senderKeySet, _, err := privatekey.GetKeySetFromPrivateKeyParams(privateKey)
	if err != nil {
		return err
	}

	PRVIDStr := "0000000000000000000000000000000000000000000000000000000000000004"
	MAX_LOOP_TIME := 100

	for {
		plainCoinsInput, _, err := services.GetListUTXO(rpcClient, PRVIDStr, senderKeySet, 2, true)
		if err != nil {
			return err
		}
		if len(plainCoinsInput) >= minNumUTXOs {
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

		fmt.Printf("Number of current coins: %v, Min Value: %v\n", len(plainCoinsInput), avgValue)
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

		MAX_LOOP_TIME--
		if MAX_LOOP_TIME == 0 {
			break
		}

		time.Sleep(1 * time.Minute)
	}
	return nil
}