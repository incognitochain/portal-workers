package workers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/metadata"
	"github.com/incognitochain/portal-workers/utils"
)

func (b *BTCBroadcastingManager) isTimeoutBTCTx(tx *BroadcastTx, curBlockHeight uint64) bool {
	broadcastBlockHeight := tx.BlkHeight

	if curBlockHeight-broadcastBlockHeight < TimeoutBTCFeeReplacement {
		return false
	}

	bcyTx, err := b.bcy.GetTX(tx.TxHash, nil)
	return err != nil || bcyTx.Confirmations <= 0 || bcyTx.BlockHeight <= 0
}

// return boolean value of transaction confirmation and bitcoin block height
func (b *BTCBroadcastingManager) isConfirmedBTCTx(txHash string) (bool, uint64) {
	tx, err := b.bcy.GetTX(txHash, nil)
	if err != nil {
		return false, 0
	}
	return tx.Confirmations >= BTCConfirmationThreshold, uint64(tx.BlockHeight)
}

func (b *BTCBroadcastingManager) broadcastTx(txContent string) error {
	_, err := b.bcy.PushTX(txContent)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not broadcast tx content %v to BTC chain - with err: %v \n", txContent, err))
		return err
	}
	return nil
}

// Return fee per unshield request and number of requests
func (b *BTCBroadcastingManager) getLatestBatchInfo(batch *entities.ProcessedUnshieldRequestBatch) (uint, uint) {
	maxFee := uint(0)
	for _, fee := range batch.ExternalFees {
		if fee > maxFee {
			maxFee = fee
		}
	}
	return maxFee, uint(len(batch.UnshieldsID))
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

func (b *BTCBroadcastingManager) getLatestBTCBlockHashFromIncog() (uint64, error) {
	params := []interface{}{}
	var btcRelayingBestStateRes entities.BTCRelayingBestStateRes
	err := b.RPCClient.RPCCall("getbtcrelayingbeststate", params, &btcRelayingBestStateRes)
	if err != nil {
		return 0, err
	}
	if btcRelayingBestStateRes.RPCError != nil {
		b.Logger.Errorf("getLatestBTCBlockHashFromIncog: call RPC error, %v\n", btcRelayingBestStateRes.RPCError.StackTrace)
		return 0, errors.New(btcRelayingBestStateRes.RPCError.Message)
	}

	// check whether there was a fork happened or not
	btcBestState := btcRelayingBestStateRes.Result
	if btcBestState == nil {
		return 0, errors.New("BTC relaying best state is nil")
	}
	currentBTCBlkHeight := btcBestState.Height
	return uint64(currentBTCBlkHeight), nil
}

func (b *BTCBroadcastingManager) getBroadcastTxsFromBeaconHeight(processedBatchIDs map[string]bool, height uint64, curIncBlkHeight uint64) (map[string]*BroadcastTx, error) {
	txArray := map[string]*BroadcastTx{}
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
		_, exists := processedBatchIDs[batch.BatchID]
		if !exists {
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
				b.Logger.Errorf("getportalsignedrawtransaction: call RPC error, %v\n", signedRawTxRes.RPCError.StackTrace)
				return txArray, errors.New(signedRawTxRes.RPCError.Message)
			}

			btcTxContent := signedRawTxRes.Result.SignedTx
			btcTxHash := signedRawTxRes.Result.TxID

			feePerRequest, numberRequest := b.getLatestBatchInfo(batch)
			txArray[batch.BatchID] = &BroadcastTx{
				TxContent:     btcTxContent,
				TxHash:        btcTxHash,
				FeePerRequest: feePerRequest,
				NumOfRequests: numberRequest,
				BlkHeight:     curIncBlkHeight,
			}
		}
	}
	return txArray, nil
}

func (b *BTCBroadcastingManager) getBroadcastReplacementTx(feeReplacementTxArray map[string]*FeeReplacementTx, curIncBlkHeight uint64) (map[string]*FeeReplacementTx, map[string]*BroadcastTx, error) {
	var params []interface{}
	txArray := map[string]*BroadcastTx{}

	for batchID, tx := range feeReplacementTxArray {
		params = []interface{}{
			map[string]string{
				"TxID": tx.ReqTxID,
			},
		}
		var signedRawTxRes entities.SignedRawTxRes
		err := b.RPCClient.RPCCall("getporalsignedrawreplacefeetransaction", params, &signedRawTxRes)
		if err != nil {
			return feeReplacementTxArray, txArray, err
		}
		if signedRawTxRes.RPCError == nil {
			btcTxContent := signedRawTxRes.Result.SignedTx
			btcTxHash := signedRawTxRes.Result.TxID

			txArray[batchID] = &BroadcastTx{
				TxContent:     btcTxContent,
				TxHash:        btcTxHash,
				FeePerRequest: tx.FeePerRequest,
				NumOfRequests: tx.NumOfRequests,
				BlkHeight:     curIncBlkHeight,
			}

			delete(feeReplacementTxArray, batchID)
		}
	}

	return feeReplacementTxArray, txArray, nil
}

func (b *BTCBroadcastingManager) buildProof(txID string, blkHeight uint64) (string, error) {
	cypherBlock, err := b.bcy.GetBlock(
		int(blkHeight),
		"",
		map[string]string{
			"txstart": "0",
			"limit":   "500",
		},
	)

	if err != nil {
		return "", err
	}

	txIDs := cypherBlock.TXids
	txHashes := make([]*chainhash.Hash, len(txIDs))
	for i := 0; i < len(txIDs); i++ {
		txHashes[i], _ = chainhash.NewHashFromStr(txIDs[i])
	}

	msgTx := utils.BuildMsgTxFromCypher(txID, b.GetNetwork())
	txHash := msgTx.TxHash()
	blkHash, _ := chainhash.NewHashFromStr(cypherBlock.Hash)

	merkleProofs := utils.BuildMerkleProof(txHashes, &txHash)
	btcProof := utils.BTCProof{
		MerkleProofs: merkleProofs,
		BTCTx:        msgTx,
		BlockHash:    blkHash,
	}
	btcProofBytes, _ := json.Marshal(btcProof)
	btcProofStr := base64.StdEncoding.EncodeToString(btcProofBytes)

	return btcProofStr, nil
}

func (b *BTCBroadcastingManager) submitConfirmedTx(proof string, batchID string) (string, error) {
	rpcClient, keyWallet, err := initSetParams(b.RPCClient)
	if err != nil {
		return "", err
	}
	meta, _ := metadata.NewPortalSubmitConfirmedTxRequest(PortalSubmitConfirmedTxMeta, proof, BTCID, batchID)
	return sendTx(rpcClient, keyWallet, meta)
}

func (b *BTCBroadcastingManager) getSubmitConfirmedTxStatus(txID string) error {
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
			if confirmedTxStatusRes.Result.Status == 1 {
				return nil
			} else {
				return fmt.Errorf("Submit confirmed transaction failed")
			}
		}
	}

	if err != nil {
		return err
	} else {
		return fmt.Errorf(confirmedTxStatusRes.RPCError.Message)
	}
}

func (b *BTCBroadcastingManager) requestFeeReplacement(batchID string, newFee uint) (string, error) {
	rpcClient, keyWallet, err := initSetParams(b.RPCClient)
	if err != nil {
		return "", err
	}
	meta, _ := metadata.NewPortalReplacementFeeRequest(PortalReplacementFeeRequestMeta, BTCID, batchID, newFee)
	return sendTx(rpcClient, keyWallet, meta)
}

func (b *BTCBroadcastingManager) getRequestFeeReplacementTxStatus(txID string) error {
	params := []interface{}{
		map[string]string{
			"ReqTxID": txID,
		},
	}

	var feeReplacementStatusRes entities.RequestStatusRes

	var err error
	for idx := 0; idx < NUM_GET_STATUS_TRIES; idx++ {
		time.Sleep(INTERVAL_TRIES)
		err = b.RPCClient.RPCCall("getportalreplacementfeestatus", params, &feeReplacementStatusRes)
		if err == nil && feeReplacementStatusRes.RPCError == nil {
			if feeReplacementStatusRes.Result.Status == 1 {
				return nil
			} else {
				return fmt.Errorf("Request fee replacement failed")
			}
		}
	}

	if err != nil {
		return err
	} else {
		return fmt.Errorf(feeReplacementStatusRes.RPCError.Message)
	}
}

func joinTxArray(array1 map[string]*BroadcastTx, array2 map[string]*BroadcastTx) map[string]*BroadcastTx {
	joinedArray := map[string]*BroadcastTx{}
	for batchID, value := range array1 {
		joinedArray[batchID] = value
	}
	for batchID, value := range array2 {
		joinedArray[batchID] = value
	}
	return joinedArray
}
