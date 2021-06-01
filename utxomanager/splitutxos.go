package utxomanager

import (
	"fmt"
	"os"
	"sync"
	"time"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/inc-backend/go-incognito/publish/transformer"
	"github.com/incognitochain/portal-workers/utils"
)

const (
	MaxLoopTime   = 100
	MaxReceiver   = 10
	MinUTXOAmount = 10000
	PRVIDStr      = "0000000000000000000000000000000000000000000000000000000000000004"
)

func SplitUTXOs(
	endpointUri string, protocol string, privateKey string, paymentAddress string, minNumUTXOs int,
	utxoManager *UTXOCache, httpClient *utils.HttpClient,
) error {
	publicIncognito := go_incognito.NewPublicIncognito(
		endpointUri,
		os.Getenv("INCOGNITO_COINSERVICE_URL"),
	)
	blockInfo := go_incognito.NewBlockInfo(publicIncognito)
	wallet := go_incognito.NewWallet(publicIncognito, blockInfo)

	// rpcClient := httpclient.NewHttpClient(endpointUri, os.Getenv("INCOGNITO_COINSERVICE_URL"), protocol, endpointUri, 0)
	// txService := services.NewTxService(rpcClient, 2)

	cntLoop := 0

	for {
		utxos, err := GetListUnspentUTXO(wallet, privateKey, utxoManager, httpClient)
		if err != nil {
			return err
		}

		fmt.Printf("Number of UTXOs: %v\n", len(utxos))

		if len(utxos) >= minNumUTXOs {
			fmt.Printf("Split UTXOs succeed.\n")
			break
		}
		// if len(utxos) == 0 {
		// 	return fmt.Errorf("Could not get any UTXO from this account")
		// }

		var wg sync.WaitGroup
		for idx := range utxos {
			utxo := utxos[idx]
			if utxo.Amount < MinUTXOAmount*MaxReceiver {
				continue
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				var receivers map[string]uint64
				// for idx := 0; idx < MaxReceiver-1; idx++ {
				// 	otaAddress, _, err := txService.GenerateOTAFromPaymentAddress(paymentAddress)
				// 	if err != nil {
				// 		return
				// 	}
				// 	receivers[otaAddress] = utxo.Amount / MaxReceiver
				// }
				receivers = map[string]uint64{
					paymentAddress: utxo.Amount / 2,
				}
				result, err := wallet.SendPrvWithUTXO(
					privateKey,
					receivers,
					[]string{utxo.KeyImage},
				)
				if err != nil {
					return
				}
				resp, err := publicIncognito.SubmitRawData(result)
				if err != nil {
					return
				}
				txID, err := transformer.TransformersTxHash(resp)
				if err != nil {
					return
				}
				CacheSpentUTXOs(privateKey, txID, []UTXO{utxo}, utxoManager)
				fmt.Printf("TxID: %+v\n", txID)
			}()
		}
		wg.Wait()

		cntLoop++
		if cntLoop >= MaxLoopTime {
			break
		}

		time.Sleep(15 * time.Second)
	}
	return nil
}
