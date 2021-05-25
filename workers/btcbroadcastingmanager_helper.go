package workers

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/inc-backend/go-incognito/publish/transformer"
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
func (b *BTCBroadcastingManager) getUnshieldFeeInfo(batch *entities.ProcessedUnshieldRequestBatch) (uint, uint) {
	minFee := uint(0)
	for _, fee := range batch.ExternalFees {
		if minFee <= 0 || fee.NetworkFee < minFee {
			minFee = fee.NetworkFee
		}
	}
	return minFee, uint(len(batch.UnshieldsID))
}

func (b *BTCBroadcastingManager) getLatestBeaconHeight() (uint64, error) {
	params := []interface{}{}
	var beaconBestStateRes entities.BeaconBestStateRes
	err := b.RPCClient.RPCCall("getbeaconbeststate", params, &beaconBestStateRes)
	if err != nil {
		return 0, err
	}

	if beaconBestStateRes.RPCError != nil {
		b.Logger.Errorf("getLatestBeaconHeight: call RPC error, %v\n", beaconBestStateRes.RPCError.StackTrace)
		return 0, errors.New(beaconBestStateRes.RPCError.Message)
	}
	return beaconBestStateRes.Result.BeaconHeight, nil
}

func (b *BTCBroadcastingManager) getBroadcastTxsFromBeaconHeight(
	broadcastTxArray map[string][]*BroadcastTx, height uint64, curIncBlkHeight uint64,
) (map[string][]*BroadcastTx, error) {
	txArray := map[string][]*BroadcastTx{}
	var params []interface{}

	params = []interface{}{
		map[string]string{
			"BeaconHeight": strconv.FormatUint(height, 10),
		},
	}
	var portalStateRes entities.PortalV4StateByHeightRes
	err := b.RPCClient.RPCCall("getportalv4state", params, &portalStateRes)
	if err != nil {
		return txArray, err
	}
	if portalStateRes.RPCError != nil {
		b.Logger.Errorf("getportalv4state: call RPC error, %v\n", portalStateRes.RPCError.StackTrace)
		return txArray, errors.New(portalStateRes.RPCError.Message)
	}

	for _, batch := range portalStateRes.Result.ProcessedUnshieldRequests[BTCID] {
		_, exists := broadcastTxArray[batch.BatchID]
		if exists {
			continue
		}
		params = []interface{}{
			map[string]string{
				"BatchID": batch.BatchID,
			},
		}
		var signedRawTxRes entities.SignedRawTxRes
		err := b.RPCClient.RPCCall("getportalsignedrawtransaction", params, &signedRawTxRes)
		if err != nil {
			return txArray, err
		}
		if signedRawTxRes.RPCError != nil {
			b.Logger.Errorf("getportalsignedrawtransaction: call RPC for batchID %v - with err %v\n", batch.BatchID, signedRawTxRes.RPCError.StackTrace)
			continue
		}

		btcTxContent := signedRawTxRes.Result.SignedTx
		btcTxHash := signedRawTxRes.Result.TxID
		vsize, err := b.getVSizeBTCTx(btcTxContent)
		if err != nil {
			continue
		}

		feePerRequest, numberRequest := b.getUnshieldFeeInfo(batch)
		acceptableFee := utils.IsEnoughFee(vsize, feePerRequest, numberRequest, b.bitcoinFee)
		txArray[batch.BatchID] = []*BroadcastTx{{
			TxContent:     btcTxContent,
			TxHash:        btcTxHash,
			VSize:         vsize,
			RBFReqTxID:    "",
			FeePerRequest: feePerRequest,
			NumOfRequests: numberRequest,
			IsBroadcasted: acceptableFee,
			BlkHeight:     curIncBlkHeight,
		},
		}
	}
	return txArray, nil
}

func (b *BTCBroadcastingManager) getLatestRBFReqTx(
	broadcastTxArray map[string][]*BroadcastTx, height uint64,
) (map[string]*entities.ExternalFeeInfo, error) {
	reqTxs := map[string]*entities.ExternalFeeInfo{}

	for batchID, txArray := range broadcastTxArray {
		status, err := b.getUnshieldingBatchStatus(batchID)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get batch %v status - with err: %v", batchID, err))
			continue
		}
		lastestReq := b.getLatestFeeInfo(status.NetworkFees)
		lastCachedReq := txArray[len(txArray)-1]
		if lastestReq.NetworkFee > lastCachedReq.FeePerRequest {
			reqTxs[batchID] = lastestReq
		}
	}
	return reqTxs, nil
}

func (b *BTCBroadcastingManager) getBroadcastReplacementTx(
	broadcastTxArray map[string][]*BroadcastTx, reqTxs map[string]*entities.ExternalFeeInfo, curIncBlkHeight uint64,
) (map[string][]*BroadcastTx, error) {
	var params []interface{}
	txArray := map[string][]*BroadcastTx{}

	for batchID, reqTx := range reqTxs {
		reqTxID := reqTx.RBFReqIncTxID
		vsize := broadcastTxArray[batchID][0].VSize
		numOfRequests := broadcastTxArray[batchID][0].NumOfRequests
		feePerRequest := reqTx.NetworkFee
		acceptableFee := utils.IsEnoughFee(vsize, feePerRequest, numOfRequests, b.bitcoinFee)
		if acceptableFee {
			params = []interface{}{
				map[string]string{
					"TxID": reqTxID,
				},
			}
			var signedRawTxRes entities.SignedRawTxRes
			err := b.RPCClient.RPCCall("getportalsignedrawreplacebyfeetransaction", params, &signedRawTxRes)
			if err != nil {
				return txArray, err
			}
			if signedRawTxRes.RPCError != nil {
				b.Logger.Errorf("getportalsignedrawreplacebyfeetransaction: call RPC for ReqTxID %v - with err %v\n", reqTxID, signedRawTxRes.RPCError.StackTrace)
				continue
			}

			btcTxContent := signedRawTxRes.Result.SignedTx
			btcTxHash := signedRawTxRes.Result.TxID

			txArray[batchID] = []*BroadcastTx{{
				TxContent:     btcTxContent,
				TxHash:        btcTxHash,
				VSize:         vsize,
				RBFReqTxID:    reqTxID,
				FeePerRequest: feePerRequest,
				NumOfRequests: numOfRequests,
				IsBroadcasted: true,
				BlkHeight:     curIncBlkHeight,
			},
			}
		} else {
			txArray[batchID] = []*BroadcastTx{{
				TxContent:     "",
				TxHash:        "",
				VSize:         vsize,
				RBFReqTxID:    reqTxID,
				FeePerRequest: feePerRequest,
				NumOfRequests: numOfRequests,
				IsBroadcasted: false,
				BlkHeight:     curIncBlkHeight,
			},
			}
		}
	}

	return txArray, nil
}

func (b *BTCBroadcastingManager) submitConfirmedTx(proof string, batchID string) (string, error) {
	metadata := map[string]interface{}{
		"PortalTokenID": BTCID,
		"BatchID":       batchID,
		"UnshieldProof": proof,
	}

	result, err := b.Portal.SubmitConfirmedTx(os.Getenv("INCOGNITO_PRIVATE_KEY"), metadata)
	if err != nil {
		return "", err
	}
	resp, err := b.Client.SubmitRawData(result)
	if err != nil {
		return "", err
	}

	txID, err := transformer.TransformersTxHash(resp)
	if err != nil {
		return "", err
	}
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
	for idx := 0; idx < NUM_GET_STATUS_TRIES; idx++ {
		time.Sleep(INTERVAL_TRIES)
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
	metadata := map[string]interface{}{
		"PortalTokenID":  BTCID,
		"BatchID":        batchID,
		"ReplacementFee": fmt.Sprintf("%v", newFee),
	}

	result, err := b.Portal.ReplaceByFee(os.Getenv("INCOGNITO_PRIVATE_KEY"), metadata)
	if err != nil {
		return "", err
	}
	resp, err := b.Client.SubmitRawData(result)
	if err != nil {
		return "", err
	}

	txID, err := transformer.TransformersTxHash(resp)
	if err != nil {
		return "", err
	}
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
	for idx := 0; idx < NUM_GET_STATUS_TRIES; idx++ {
		time.Sleep(INTERVAL_TRIES)
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

func (b *BTCBroadcastingManager) getLatestFeeInfo(feeState map[uint64]*entities.ExternalFeeInfo) *entities.ExternalFeeInfo {
	maxBlkHeight := uint64(0)
	for blkHeight := range feeState {
		if blkHeight > maxBlkHeight {
			maxBlkHeight = blkHeight
		}
	}
	return feeState[maxBlkHeight]
}

func joinTxArray(array1 map[string][]*BroadcastTx, array2 map[string][]*BroadcastTx) map[string][]*BroadcastTx {
	joinedArray := map[string][]*BroadcastTx{}
	for batchID, value := range array1 {
		_, exist := joinedArray[batchID]
		if !exist {
			joinedArray[batchID] = []*BroadcastTx{}
		}
		joinedArray[batchID] = append(joinedArray[batchID], value...)
	}
	for batchID, value := range array2 {
		_, exist := joinedArray[batchID]
		if !exist {
			joinedArray[batchID] = []*BroadcastTx{}
		}
		joinedArray[batchID] = append(joinedArray[batchID], value...)
	}
	return joinedArray
}

func getLastestBroadcastTx(txArray []*BroadcastTx) *BroadcastTx {
	lastestTx := txArray[0]
	for idx := 1; idx < len(txArray); idx++ {
		if txArray[idx].BlkHeight > lastestTx.BlkHeight {
			lastestTx = txArray[idx]
		}
	}
	return lastestTx
}
