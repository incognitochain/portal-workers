package workers

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/incognitochain/go-incognito-sdk-v2/coin"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
)

func (b *BTCBroadcastingManager) isTimeoutBTCTx(tx *BroadcastTx, curBlockHeight uint64) bool {
	broadcastBlockHeight := tx.BlkHeight

	if !tx.IsBroadcasted {
		return curBlockHeight-broadcastBlockHeight >= TimeIntervalBTCFeeReplacement
	} else {
		if curBlockHeight-broadcastBlockHeight < TimeoutBTCFeeReplacement {
			return false
		}

		txHash, _ := chainhash.NewHashFromStr(tx.TxHash)
		tx, err := b.btcClient.GetRawTransactionVerbose(txHash)
		return err != nil || tx.Confirmations <= 0
	}
}

// return boolean value of transaction confirmation and bitcoin block height
func (b *BTCBroadcastingManager) isConfirmedBTCTx(txHash string) (bool, uint64) {
	txID, _ := chainhash.NewHashFromStr(txHash)
	tx, err := b.btcClient.GetRawTransactionVerbose(txID)
	if err != nil {
		return false, 0
	}
	if tx.Confirmations >= BTCConfirmationThreshold {
		blkHash, _ := chainhash.NewHashFromStr(tx.BlockHash)
		blk, err := b.btcClient.GetBlockHeaderVerbose(blkHash)
		if err != nil {
			return false, 0
		}
		return true, uint64(blk.Height)
	}
	return false, 0
}

func (b *BTCBroadcastingManager) getVSizeBTCTx(txContent string) (int, error) {
	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txContent)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not serialized tx content %v - with err: %v \n", txContent, err))
		return 0, err
	}

	txRawResult, err := b.btcClient.DecodeRawTransaction(serializedTx)
	if err != nil {
		fmt.Printf("Could not decode tx content %v - with err: %v \n", txContent, err)
		// b.ExportErrorLog(fmt.Sprintf("Could not decode tx content %v - with err: %v \n", txContent, err))
		return 0, err
	}

	return int(txRawResult.Vsize), nil
}

func (b *BTCBroadcastingManager) broadcastTx(txContent string) error {
	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txContent)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not serialized tx content %v - with err: %v \n", txContent, err))
		return err
	}

	msgTx := &wire.MsgTx{}
	err = msgTx.Deserialize(bytes.NewReader(serializedTx))
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not deserialize tx content %v to MsgTx - with err: %v \n", txContent, err))
		return err
	}

	_, err = b.btcClient.SendRawTransaction(msgTx, true)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not broadcast tx content %v to BTC chain - with err: %v \n", txContent, err))
		return err
	}
	return nil
}

// Return fee per unshield request and number of requests
func (b *BTCBroadcastingManager) getUnshieldFeeInfo(status *entities.UnshieldingBatchStatus) (uint, uint) {
	minFee := uint(0)
	for _, fee := range status.NetworkFees {
		if minFee <= 0 || fee.NetworkFee < minFee {
			minFee = fee.NetworkFee
		}
	}
	return minFee, uint(len(status.UnshieldIDs))
}

func (b *BTCBroadcastingManager) getInitialRawTx(
	batchID string, status *entities.UnshieldingBatchStatus, curIncBlkHeight uint64,
) (*BroadcastTx, error) {
	params := []interface{}{
		map[string]string{
			"BatchID": batchID,
		},
	}
	var signedRawTxRes entities.SignedRawTxRes
	err := b.RPCClient.RPCCall("getportalsignedrawtransaction", params, &signedRawTxRes)
	if err != nil {
		b.Logger.Errorf("getportalsignedrawtransaction: call RPC for batchID %v - with err %v\n", batchID, err)
		return nil, err
	}
	if signedRawTxRes.RPCError != nil {
		b.Logger.Errorf("getportalsignedrawtransaction: call RPC for batchID %v - with err %v\n", batchID, signedRawTxRes.RPCError.StackTrace)
		return nil, err
	}

	btcTxContent := signedRawTxRes.Result.SignedTx
	btcTxHash := signedRawTxRes.Result.TxID
	vsize, err := b.getVSizeBTCTx(btcTxContent)
	if err != nil {
		b.Logger.Errorf("get vsize of tx for batchID %v - with err %v\n", batchID, err)
		return nil, err
	}

	feePerRequest, numberRequest := b.getUnshieldFeeInfo(status)
	acceptableFee := utils.IsEnoughFee(vsize, feePerRequest, numberRequest, b.bitcoinFee)
	return &BroadcastTx{
		TxContent:     btcTxContent,
		TxHash:        btcTxHash,
		VSize:         vsize,
		RBFReqTxID:    "",
		FeePerRequest: feePerRequest,
		NumOfRequests: numberRequest,
		IsBroadcasted: acceptableFee,
		BlkHeight:     curIncBlkHeight,
	}, nil
}

func (b *BTCBroadcastingManager) getRBFRawTx(
	batchID string, reqTx *entities.ExternalFeeInfo, initBroadcastTx *BroadcastTx, curIncBlkHeight uint64,
) (*BroadcastTx, error) {
	reqTxID := reqTx.RBFReqIncTxID
	vsize := initBroadcastTx.VSize
	numOfRequests := initBroadcastTx.NumOfRequests
	feePerRequest := reqTx.NetworkFee
	acceptableFee := utils.IsEnoughFee(vsize, feePerRequest, numOfRequests, b.bitcoinFee)
	if !acceptableFee && feePerRequest == MaxUnshieldFee {
		acceptableFee = true
	}

	params := []interface{}{
		map[string]string{
			"TxID": reqTxID,
		},
	}
	var signedRawTxRes entities.SignedRawTxRes
	err := b.RPCClient.RPCCall("getportalsignedrawreplacebyfeetransaction", params, &signedRawTxRes)
	if err != nil {
		b.Logger.Errorf("getportalsignedrawreplacebyfeetransaction: call RPC for ReqTxID %v - with err %v\n", reqTxID, err)
		return nil, err
	}
	if signedRawTxRes.RPCError != nil {
		b.Logger.Errorf("getportalsignedrawreplacebyfeetransaction: call RPC for ReqTxID %v - with err %v\n", reqTxID, signedRawTxRes.RPCError.StackTrace)
		return nil, err
	}

	btcTxContent := signedRawTxRes.Result.SignedTx
	btcTxHash := signedRawTxRes.Result.TxID

	return &BroadcastTx{
		TxContent:     btcTxContent,
		TxHash:        btcTxHash,
		VSize:         vsize,
		RBFReqTxID:    reqTxID,
		FeePerRequest: feePerRequest,
		NumOfRequests: numOfRequests,
		IsBroadcasted: acceptableFee,
		BlkHeight:     curIncBlkHeight,
	}, nil

}

func (b *BTCBroadcastingManager) getBroadcastTx(
	previousBroadcastTxArray map[string]map[string]*BroadcastTx, batchIDs []string, curIncBlkHeight uint64,
) (map[string]map[string]*BroadcastTx, error) {
	txArray := map[string]map[string]*BroadcastTx{}

	for _, batchID := range batchIDs {
		status, err := b.getUnshieldingBatchStatus(batchID)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get batch %v status - with err: %v", batchID, err))
			continue
		}
		txArray[batchID] = map[string]*BroadcastTx{}

		var initBroadcastTx *BroadcastTx
		for _, reqTx := range status.NetworkFees {
			if reqTx.RBFReqIncTxID == "" {
				var isExisted bool
				_, isExisted = previousBroadcastTxArray[batchID]
				if isExisted {
					_, isExisted = previousBroadcastTxArray[batchID][""]
					if isExisted {
						initBroadcastTx = previousBroadcastTxArray[batchID][""]
						continue
					}
				}

				broadcastTx, err := b.getInitialRawTx(batchID, status, curIncBlkHeight)
				if err != nil {
					return txArray, nil
				}
				txArray[batchID][""] = broadcastTx
				initBroadcastTx = broadcastTx
			}
		}

		for _, reqTx := range status.NetworkFees {
			if reqTx.RBFReqIncTxID != "" {
				var isExisted bool
				_, isExisted = previousBroadcastTxArray[batchID]
				if isExisted {
					_, isExisted = previousBroadcastTxArray[batchID][reqTx.RBFReqIncTxID]
					if isExisted {
						continue
					}
				}

				broadcastTx, err := b.getRBFRawTx(batchID, reqTx, initBroadcastTx, curIncBlkHeight)
				if err != nil {
					return txArray, err
				}
				txArray[batchID][reqTx.RBFReqIncTxID] = broadcastTx
			}
		}
	}

	return txArray, nil
}

func (b *BTCBroadcastingManager) submitConfirmedTx(proof string, batchID string) (string, error) {
	utxos, tmpTxID, err := b.UTXOManager.GetUTXOsByAmount(os.Getenv("INCOGNITO_PRIVATE_KEY"), DefaultNetworkFee)
	if err != nil {
		return "", err
	}
	utxoCoins := []coin.PlainCoin{}
	utxoIndices := []uint64{}
	for _, utxo := range utxos {
		utxoCoins = append(utxoCoins, utxo.Coin)
		utxoIndices = append(utxoIndices, utxo.Index.Uint64())
	}

	txID, err := b.UTXOManager.IncClient.CreateAndSendPortalSubmitConfirmationTransaction(
		os.Getenv("INCOGNITO_PRIVATE_KEY"),
		BTCID,
		proof,
		batchID,
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

func (b *BTCBroadcastingManager) getSubmitConfirmedTxStatus(txID string) (int, error) {
	params := []interface{}{
		map[string]string{
			"ReqTxID": txID,
		},
	}

	var confirmedTxStatusRes entities.RequestStatusRes

	var err error
	for idx := 0; idx < NumGetStatusTries; idx++ {
		time.Sleep(IntervalTries)
		err = b.RPCClient.RPCCall("getportalsubmitconfirmedtxstatus", params, &confirmedTxStatusRes)
		if err == nil && confirmedTxStatusRes.RPCError == nil {
			return confirmedTxStatusRes.Result.Status, nil
		}
	}

	if err != nil {
		return 0, err
	} else {
		return 0, fmt.Errorf(confirmedTxStatusRes.RPCError.Message)
	}
}

func (b *BTCBroadcastingManager) requestFeeReplacement(batchID string, newFee uint) (string, error) {
	utxos, tmpTxID, err := b.UTXOManager.GetUTXOsByAmount(os.Getenv("INCOGNITO_PRIVATE_KEY"), DefaultNetworkFee)
	if err != nil {
		return "", err
	}
	utxoCoins := []coin.PlainCoin{}
	utxoIndices := []uint64{}
	for _, utxo := range utxos {
		utxoCoins = append(utxoCoins, utxo.Coin)
		utxoIndices = append(utxoIndices, utxo.Index.Uint64())
	}

	txID, err := b.UTXOManager.IncClient.CreateAndSendPortalReplaceByFeeTransaction(
		os.Getenv("INCOGNITO_PRIVATE_KEY"),
		BTCID,
		batchID,
		newFee,
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

func (b *BTCBroadcastingManager) getRequestFeeReplacementTxStatus(txID string) (int, error) {
	params := []interface{}{
		map[string]string{
			"ReqTxID": txID,
		},
	}

	var feeReplacementStatusRes entities.RequestStatusRes

	var err error
	for idx := 0; idx < NumGetStatusTries; idx++ {
		time.Sleep(IntervalTries)
		err = b.RPCClient.RPCCall("getportalreplacebyfeestatus", params, &feeReplacementStatusRes)
		if err == nil && feeReplacementStatusRes.RPCError == nil {
			return feeReplacementStatusRes.Result.Status, nil
		}
	}

	if err != nil {
		return 0, err
	} else {
		return 0, fmt.Errorf(feeReplacementStatusRes.RPCError.Message)
	}
}

func (b *BTCBroadcastingManager) getUnshieldingBatchStatus(batchID string) (*entities.UnshieldingBatchStatus, error) {
	params := []interface{}{
		map[string]string{
			"BatchID": batchID,
		},
	}

	var unshieldingBatchStatusRes entities.UnshieldingBatchStatusRes

	err := b.RPCClient.RPCCall("getportalbatchunshieldrequeststatus", params, &unshieldingBatchStatusRes)
	if err == nil && unshieldingBatchStatusRes.RPCError == nil {
		return unshieldingBatchStatusRes.Result, nil
	}

	if err != nil {
		return nil, err
	} else {
		return nil, fmt.Errorf(unshieldingBatchStatusRes.RPCError.Message)
	}
}

func getLastestBroadcastTx(txArray map[string]*BroadcastTx) *BroadcastTx {
	lastestTx := txArray[""]
	for _, tx := range txArray {
		if tx.FeePerRequest > lastestTx.FeePerRequest {
			lastestTx = tx
		}
	}
	return lastestTx
}

func joinTxArray(
	array1 map[string]map[string]*BroadcastTx, array2 map[string]map[string]*BroadcastTx,
) map[string]map[string]*BroadcastTx {
	joinedArray := map[string]map[string]*BroadcastTx{}
	for batchID, batchContent := range array1 {
		_, exist := joinedArray[batchID]
		if !exist {
			joinedArray[batchID] = map[string]*BroadcastTx{}
		}
		for rbfTxID, value := range batchContent {
			joinedArray[batchID][rbfTxID] = value
		}
	}
	for batchID, batchContent := range array2 {
		_, exist := joinedArray[batchID]
		if !exist {
			joinedArray[batchID] = map[string]*BroadcastTx{}
		}
		for rbfTxID, value := range batchContent {
			joinedArray[batchID][rbfTxID] = value
		}
	}
	return joinedArray
}
