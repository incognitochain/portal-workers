package workers

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcutil"
	"github.com/incognitochain/go-incognito-sdk-v2/coin"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"
)

const (
	FirstConvertTxBTCHeight = int32(697490)
)

type BTCConvertVaultRequestSender struct {
	WorkerAbs
	btcClient *rpcclient.Client
	chainCfg  *chaincfg.Params
}

func (b *BTCConvertVaultRequestSender) Init(id int, name string, freq int, network string, utxoManager *utxomanager.UTXOManager) error {
	b.WorkerAbs.Init(id, name, freq, network, utxoManager)

	var err error

	// init bitcoin rpcclient
	b.btcClient, err = utils.BuildBTCClient()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not initialize Bitcoin RPCClient - with err: %v", err))
		return err
	}

	b.chainCfg = &chaincfg.TestNet3Params
	if b.Network == "main" {
		b.chainCfg = &chaincfg.MainNetParams
	} else if b.Network == "reg" {
		b.chainCfg = &chaincfg.RegressionNetParams
	}

	return nil
}

func (b *BTCConvertVaultRequestSender) ExportErrorLog(msg string) {
	b.WorkerAbs.ExportErrorLog(msg)
}

func (b *BTCConvertVaultRequestSender) ExportInfoLog(msg string) {
	b.WorkerAbs.ExportInfoLog(msg)
}

func (b *BTCConvertVaultRequestSender) Execute() {
	b.ExportErrorLog("BTCConvertVaultRequestSender worker is executing...")

	address, err := btcutil.DecodeAddress(os.Getenv("BTC_VAULT_ADDRESS"), b.chainCfg)
	if err != nil {
		panic("Could not decode BTC vault address")
	}
	trackingAddresses := []btcutil.Address{address}

	lastScannedBTCBlockHeight := FirstConvertTxBTCHeight - 1
	shardID, _ := strconv.Atoi(os.Getenv("SHARD_ID"))

	for {
		minConfirmation := BTCConfirmationThreshold
		maxConfirmation := 99999999
		listUnspentResults, err := b.btcClient.ListUnspentMinMaxAddresses(minConfirmation, maxConfirmation, trackingAddresses)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not scan list of unspent outcoins - with err: %v", err))
			return
		}

		relayingBTCHeight, err := getLatestBTCHeightFromIncog(b.RPCBTCRelayingReaders)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}

		curBTCBlockHeight := int32(0)
		var wg sync.WaitGroup
		for _, unspentCoins := range listUnspentResults {
			txID, _ := chainhash.NewHashFromStr(unspentCoins.TxID)
			tx, err := b.btcClient.GetRawTransactionVerbose(txID)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not get raw tx %v - with err: %v", txID, err))
				return
			}
			blkHash, _ := chainhash.NewHashFromStr(tx.BlockHash)
			blk, err := b.btcClient.GetBlockHeaderVerbose(blkHash)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not get block %v - with err: %v", tx.BlockHash, err))
				return
			}

			if blk.Height <= lastScannedBTCBlockHeight || blk.Height+BTCConfirmationThreshold-1 > int32(relayingBTCHeight) {
				continue
			}
			if blk.Height > curBTCBlockHeight {
				curBTCBlockHeight = blk.Height
			}

			fmt.Printf("Create Converting Vault Request for BTC Tx %v\n", txID)

			// Build proof and send request by go routines
			wg.Add(1)
			go func() {
				defer wg.Done()
				// build proof
				proof, err := utils.BuildProof(b.btcClient, tx.Hash, uint64(blk.Height))
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not build proof for tx: %v - with err: %v", tx.Hash, err))
					return
				}

				// send convert vault request
				txID, err := b.submitConvertVaultRequest(proof)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not send converting request from BTC tx %v proof with err: %v", tx.Hash, err))
					return
				}
				fmt.Printf("Shielding txID: %v\n", txID)

				// get status
				status, errorStr, err := b.getRequestConvertVaultStatus(txID)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not get request converting status from BTC tx %v - with err: %v", tx.Hash, err))
				} else {
					ok := isFinalizedTx(b.UTXOManager.IncClient, b.Logger, shardID, txID)
					if !ok {
						return
					}
					if status == 0 { // rejected
						if errorStr == "IsExistedProof" {
							b.ExportErrorLog(fmt.Sprintf("Request converting failed BTC tx %v, converting txID %v - duplicated proof", tx.Hash, txID))
						} else {
							b.ExportErrorLog(fmt.Sprintf("Request converting failed BTC tx %v, converting txID %v - invalid proof", tx.Hash, txID))
						}
					} else {
						b.ExportInfoLog(fmt.Sprintf("Request conveting succeed BTC tx %v, converting txID %v", tx.Hash, txID))
					}

				}
			}()
		}
		wg.Wait()
		if curBTCBlockHeight > 0 {
			lastScannedBTCBlockHeight = curBTCBlockHeight
		}

		time.Sleep(100 * time.Second)
	}
}

func (b *BTCConvertVaultRequestSender) submitConvertVaultRequest(proof string) (string, error) {
	utxos, tmpTxID, err := b.UTXOManager.GetUTXOsByAmount(os.Getenv("INCOGNITO_PRIVATE_KEY"), 5000)
	if err != nil {
		return "", err
	}
	utxoCoins := []coin.PlainCoin{}
	utxoIndices := []uint64{}
	for _, utxo := range utxos {
		utxoCoins = append(utxoCoins, utxo.Coin)
		utxoIndices = append(utxoIndices, utxo.Index.Uint64())
	}

	txID, err := b.UTXOManager.IncClient.CreateAndSendPortalConvertVaultTransaction(
		os.Getenv("INCOGNITO_PRIVATE_KEY"),
		BTCID,
		proof,
		utxoCoins,
		utxoIndices,
	)
	if err != nil {
		b.UTXOManager.UncachedUTXOByTmpTxID(os.Getenv("INCOGNITO_PRIVATE_KEY"), tmpTxID)
		return "", err
	}
	b.UTXOManager.UpdateTxID(os.Getenv("INCOGNITO_PRIVATE_KEY"), tmpTxID, txID)
	return txID, nil
}

func (b *BTCConvertVaultRequestSender) getRequestConvertVaultStatus(txID string) (int, string, error) {
	params := []interface{}{
		map[string]string{
			"ReqTxID": txID,
		},
	}

	var convertingRequestStatusRes entities.ConvertingVauleRequestStatusRes

	var err error
	for idx := 0; idx < NumGetStatusTries; idx++ {
		time.Sleep(IntervalTries)
		err = b.RPCClient.RPCCall("getportalconvertvaultstatus", params, &convertingRequestStatusRes)
		if err == nil && convertingRequestStatusRes.RPCError == nil {
			return convertingRequestStatusRes.Result.Status, convertingRequestStatusRes.Result.ErrorMsg, nil
		}
	}

	if err != nil {
		return 0, "", err
	} else {
		return 0, "", fmt.Errorf(convertingRequestStatusRes.RPCError.Message)
	}
}
