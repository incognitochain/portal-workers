package workers

import (
	"fmt"
	"os"
	"time"

	"github.com/0xkraken/btcd/btcjson"
	"github.com/0xkraken/btcd/chaincfg"
	"github.com/0xkraken/btcd/chaincfg/chainhash"
	"github.com/0xkraken/btcd/rpcclient"
	"github.com/0xkraken/btcutil"
	"github.com/incognitochain/go-incognito-sdk-v2/coin"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"
	"github.com/syndtr/goleveldb/leveldb"
)

type RescanShieldReqWorker struct {
	WorkerAbs
	btcClient *rpcclient.Client
	chainCfg  *chaincfg.Params
	db        *leveldb.DB
}

func (b *RescanShieldReqWorker) Init(id int, name string, freq int, network string, utxoManager *utxomanager.UTXOManager) error {
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

func (b *RescanShieldReqWorker) ExportErrorLog(msg string) {
	b.WorkerAbs.ExportErrorLog(msg)
}

func (b *RescanShieldReqWorker) ExportInfoLog(msg string) {
	b.WorkerAbs.ExportInfoLog(msg)
}

func (b *RescanShieldReqWorker) submitShieldingRequest(incAddress string, proof string) (string, error) {
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

	txID, err := b.UTXOManager.IncClient.CreateAndSendPortalShieldTransaction(
		os.Getenv("INCOGNITO_PRIVATE_KEY"),
		BTCID,
		incAddress,
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

func (b *RescanShieldReqWorker) buildProofAndSubmit(utxo btcjson.ListUnspentResult, incAddress string) (string, error) {
	txHash := utxo.TxID

	txID, _ := chainhash.NewHashFromStr(txHash)
	tx, _ := b.btcClient.GetRawTransactionVerbose(txID)
	blkHash, _ := chainhash.NewHashFromStr(tx.BlockHash)
	blk, err := b.btcClient.GetBlockHeaderVerbose(blkHash)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get block height of block hash: %v for txID: %v - with err: %v", blkHash, txID, err))
		return "", err
	}

	// generate proof
	proof, err := utils.BuildProof(b.btcClient, txHash, uint64(blk.Height))
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not build proof for tx: %v - with err: %v", txHash, err))
		return "", err
	}

	b.ExportInfoLog(fmt.Sprintf("Found shielding request for address %v, with BTC tx %v\n", incAddress, txHash))

	// send RPC
	txShieldID, err := b.submitShieldingRequest(incAddress, proof)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not send shielding request from BTC tx %v proof with err: %v", txHash, err))
		return "", err
	}

	b.ExportInfoLog(fmt.Sprintf("Shielding TxID %v, with BTC tx %v\n", txShieldID, txHash))
	return txShieldID, nil
}

func (b *RescanShieldReqWorker) getRequestShieldingStatus(txID string) (int, string, error) {
	params := []interface{}{
		map[string]string{
			"ReqTxID": txID,
		},
	}

	var requestShieldingStatusRes entities.ShieldingRequestStatusRes

	var err error
	for idx := 0; idx < NumGetStatusTries; idx++ {
		err = b.RPCClient.RPCCall("getportalshieldingrequeststatus", params, &requestShieldingStatusRes)
		if err == nil && requestShieldingStatusRes.RPCError == nil {
			return requestShieldingStatusRes.Result.Status, requestShieldingStatusRes.Result.Error, nil
		}
	}

	if err != nil {
		return 0, "", err
	} else {
		return 0, "", fmt.Errorf(requestShieldingStatusRes.RPCError.Message)
	}
}

// This function will execute a worker that has 2 main tasks:
// - Monitor Bitcoin multisig wallets that corresponding with Incognito Wallet App users
// - Send shielding request on behalf of users to Incognito chain
func (b *RescanShieldReqWorker) Execute() {
	b.ExportErrorLog("RescanShieldReqWorker worker is executing...")

	isBTCNodeAlive := getBTCFullnodeStatus(b.btcClient)
	if !isBTCNodeAlive {
		b.ExportErrorLog("Could not connect to BTC full node")
		return
	}

	// get list all shielding addresses from portal backend
	currentTimeStamp := time.Now().Unix()
	newlyTrackingInstance, err := GetListShieldingAddress(1577862000, currentTimeStamp)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get tracking instance from API - with err: %v", err))
		return
	}

	// newlyTrackingInstance := []ShieldingMonitoringInfo{
	// 	{
	// 		IncAddress: "12svxAhDa8x9z71FZWc2YMMaH1qHhibJbD2TpVFAetmKYR2R8L211YCEuYyvF7KKkMgJvzoyomiDketLeVrCQ7dSteBGHUNCMwnVjiW2GEohd66n3BW1YXjwhETRfL8HNhqRD8heJcgDBeNgEjsQ",
	// 		BTCAddress: "bc1qteqsgk5zww955wt0k993h637wml5j5ks7ad4065tdutszw8gyn7s7jlhlk",
	// 	},
	// }

	trackingBTCAddresses := []btcutil.Address{}
	addrMap := map[string]string{} // BTC address => Inc address
	for _, instance := range newlyTrackingInstance {
		address, err := btcutil.DecodeAddress(instance.BTCAddress, b.chainCfg)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not decode address %v - with err: %v", instance.BTCAddress, err))
			continue
		}
		trackingBTCAddresses = append(trackingBTCAddresses, address)
		addrMap[instance.BTCAddress] = instance.IncAddress
	}

	// get list utxos of shielding addresses
	minConfirmation := 6
	maxConfirmation := 9999999
	listUnspentResults, err := b.btcClient.ListUnspentMinMaxAddresses(minConfirmation, maxConfirmation, trackingBTCAddresses)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not scan list of unspent outcoins - with err: %v", err))
		return
	}

	// get portal state from fullnode to extract all UTXOs
	// TODO: update new beacon height before running
	beaconHeight := uint64(2269289)
	portalState, err := GetPortalState(beaconHeight, b.RPCClient)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get portal v4 state - with err: %v", err))
		return
	}
	utxosState := portalState.UTXOs[BTCID]

	// re-submit shielding proof if the UTXOs doesn't exist in UTXOs state
	countResubmitSuccess := uint(0)
	countResubmitError := uint(0)
	countResubmitted := uint(0)
	countSubmit := uint(0)

	type TxIDRes struct {
		incTxID string
		btcTxID string
	}

	txIDs := []TxIDRes{}

	for _, u := range listUnspentResults {
		uK := GenerateUTXOObjectKey(BTCID, u.Address, u.TxID, u.Vout)

		// submitted
		if utxosState[uK.String()] != nil {
			continue
		}

		// re-submit
		txId, err := b.buildProofAndSubmit(u, addrMap[u.Address])
		countSubmit++
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Error when build proof and submit txID %v: %v\n", u.TxID, err))
			countResubmitError++
		} else {
			txIDs = append(txIDs, TxIDRes{
				incTxID: txId,
				btcTxID: u.TxID,
			})
		}
	}

	time.Sleep(IntervalTries)

	for _, item := range txIDs {
		incTxID := item.incTxID
		status, errStr, err := b.getRequestShieldingStatus(incTxID)
		if err != nil {
			b.ExportInfoLog(fmt.Sprintf("Can not get shielding status txId %v - %v - %v\n", incTxID, err, errStr))
		}

		if status == 0 { // rejected
			if errStr == "IsExistedProof" {
				b.ExportInfoLog(fmt.Sprintf("Duplicate proof %v\n", incTxID))
				countResubmitted++
			} else {
				b.ExportErrorLog(fmt.Sprintf("shielding txID %v btcTxID %v - invalid proof\n", incTxID, item.btcTxID))
				countResubmitError++
			}
		} else {
			countResubmitSuccess++
		}
	}

	b.ExportInfoLog(fmt.Sprintf("countResubmitSuccess %v\n", countResubmitSuccess))
	b.ExportInfoLog(fmt.Sprintf("countResubmitted %v\n", countResubmitted))
	b.ExportInfoLog(fmt.Sprintf("countResubmitError %v\n", countResubmitError))
	b.ExportInfoLog(fmt.Sprintf("countSubmit %v\n", countSubmit))

	return
}
