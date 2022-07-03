package workers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/incognitochain/go-incognito-sdk-v2/incclient"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/sirupsen/logrus"
)

type btcBestStateRes struct {
	btcBestState *blockchain.BestState
	err          error
}

func getBTCFullnodeStatus(btcClient *rpcclient.Client) bool {
	_, err := btcClient.GetBlockChainInfo()
	return err == nil
}

func getBTCBestStateFromIncog(rpcRelayingReaders []*utils.HttpClient) (*blockchain.BestState, error) {
	var wg sync.WaitGroup
	btcBestStates := make(chan btcBestStateRes, len(rpcRelayingReaders))
	for _, btcRelayingHeader := range rpcRelayingReaders {
		rpcClient := btcRelayingHeader
		wg.Add(1)
		go func() {
			defer wg.Done()
			params := []interface{}{}
			var btcRelayingBestStateRes entities.BTCRelayingBestStateRes
			err := rpcClient.RPCCall("getbtcrelayingbeststate", params, &btcRelayingBestStateRes)
			if err != nil {
				btcBestStates <- btcBestStateRes{btcBestState: nil, err: err}
				return
			}
			if btcRelayingBestStateRes.RPCError != nil {
				btcBestStates <- btcBestStateRes{btcBestState: nil, err: errors.New(btcRelayingBestStateRes.RPCError.Message)}
				return
			}
			btcBestState := btcRelayingBestStateRes.Result
			if btcBestState == nil {
				btcBestStates <- btcBestStateRes{btcBestState: nil, err: errors.New("BTC relaying best state is nil")}
				return
			}
			btcBestStates <- btcBestStateRes{btcBestState: btcBestState, err: nil}
		}()
	}
	wg.Wait()

	close(btcBestStates)

	lowestHeight := int32(-1)
	var lowestBestState *blockchain.BestState
	for btcBestStateRes := range btcBestStates {
		if btcBestStateRes.err == nil && (lowestHeight == -1 || btcBestStateRes.btcBestState.Height < lowestHeight) {
			lowestHeight = btcBestStateRes.btcBestState.Height
			lowestBestState = btcBestStateRes.btcBestState
		}
	}
	if lowestHeight < 0 {
		return nil, errors.New("Can not get height from all beacon and fullnode")
	}

	return lowestBestState, nil
}

func getLatestBTCHeightFromIncog(rpcRelayingReaders []*utils.HttpClient) (uint64, error) {
	btcBestState, err := getBTCBestStateFromIncog(rpcRelayingReaders)
	if err != nil {
		return 0, err
	}
	currentBTCBlkHeight := btcBestState.Height
	return uint64(currentBTCBlkHeight), nil
}

func getLatestBTCHeightFromIncogWithoutFork(
	btcClient *rpcclient.Client, rpcRelayingReaders []*utils.HttpClient, logger *logrus.Entry,
) (uint64, error) {
	btcBestState, err := getBTCBestStateFromIncog(rpcRelayingReaders)
	if err != nil {
		return 0, err
	}

	currentBTCBlkHashStr := btcBestState.Hash.String()
	currentBTCBlkHeight := uint64(btcBestState.Height)

	blkHash, err := btcClient.GetBlockHash(int64(currentBTCBlkHeight))
	if err != nil {
		return 0, err
	}

	if blkHash.String() != currentBTCBlkHashStr { // fork detected
		msg := fmt.Sprintf("There was a fork happened at block %d, stepping back %d blocks now...", currentBTCBlkHeight, BlockStepBacks)
		logger.Warnf(msg)
		utils.SendSlackNotification(msg, utils.AlertNotification)
		return currentBTCBlkHeight - BlockStepBacks, nil
	}
	return currentBTCBlkHeight, nil
}

func getFinalizedShardHeight(incClient *incclient.IncClient, logger *logrus.Entry, shardID int) (uint64, error) {
	params := []interface{}{
		shardID,
	}

	resp, err := incClient.NewRPCCall("1.0", "getallviewdetail", params, 1)
	if err != nil {
		return 0, err
	}

	var allViewRes entities.AllViewRes
	json.Unmarshal(resp, &allViewRes)

	if allViewRes.RPCError != nil {
		logger.Errorf("getFinalizedBeaconHeight: call RPC error, %v\n", allViewRes.RPCError.StackTrace)
		return 0, errors.New(allViewRes.RPCError.Message)
	}

	finalViewHeight := uint64(0)
	for _, view := range allViewRes.Result {
		if finalViewHeight == 0 || view.Height < finalViewHeight {
			finalViewHeight = view.Height
		}
	}

	return finalViewHeight, nil
}

func isFinalizedTx(incClient *incclient.IncClient, logger *logrus.Entry, shardID int, txID string) bool {
	for idx := 0; idx < NumGetStatusTries; idx++ {
		txDetail, err := incClient.GetTxDetail(txID)
		if err != nil {
			time.Sleep(IntervalTries)
			continue
		}

		currentFinalizedHeight, err := getFinalizedShardHeight(incClient, logger, shardID)
		if err != nil {
			time.Sleep(IntervalTries)
			continue
		}

		if currentFinalizedHeight >= txDetail.BlockHeight {
			return true
		}

		time.Sleep(IntervalTries)
	}

	return false
}

func getFirstBroadcastHeight(batch *entities.ProcessedUnshieldRequestBatch) uint64 {
	var firstBroadcastHeight uint64
	for blkHeight := range batch.ExternalFees {
		if firstBroadcastHeight == 0 {
			firstBroadcastHeight = blkHeight
		} else {
			if firstBroadcastHeight > blkHeight {
				firstBroadcastHeight = blkHeight
			}
		}
	}
	return firstBroadcastHeight
}

func getBatchIDsFromBeaconHeight(height uint64, rpcClient *utils.HttpClient, logger *logrus.Entry, firstScannedBlockHeight uint64) ([]string, error) {
	batchIDs := []string{}

	params := []interface{}{
		map[string]string{
			"BeaconHeight": strconv.FormatUint(height, 10),
		},
	}
	var portalStateRes entities.PortalV4StateByHeightRes
	err := rpcClient.RPCCall("getportalv4state", params, &portalStateRes)
	if err != nil {
		return batchIDs, err
	}
	if portalStateRes.RPCError != nil {
		logger.Errorf("getportalv4state: call RPC error, %v\n", portalStateRes.RPCError.StackTrace)
		return batchIDs, errors.New(portalStateRes.RPCError.Message)
	}

	for _, batch := range portalStateRes.Result.ProcessedUnshieldRequests[BTCID] {
		if getFirstBroadcastHeight(batch) >= firstScannedBlockHeight {
			batchIDs = append(batchIDs, batch.BatchID)
		}
	}
	return batchIDs, nil
}

func getPortalStateFromBeaconHeight(
	height uint64, rpcClient *utils.HttpClient, logger *logrus.Entry) (*entities.PortalV4State,	error) {
	params := []interface{}{
		map[string]string{
			"BeaconHeight": strconv.FormatUint(height, 10),
		},
	}
	var portalStateRes entities.PortalV4StateByHeightRes
	err := rpcClient.RPCCall("getportalv4state", params, &portalStateRes)
	if err != nil {
		return nil, err
	}
	if portalStateRes.RPCError != nil {
		logger.Errorf("getportalv4state: call RPC error, %v\n", portalStateRes.RPCError.StackTrace)
		return nil, errors.New(portalStateRes.RPCError.Message)
	}
	return portalStateRes.Result, nil
}

func getPTokenAmount(rpcClient *utils.HttpClient, logger *logrus.Entry, tokenID string) (uint64, error) {
	params := []interface{}{
		map[string]string{},
	}
	var bridgeTokenInfoRes entities.BridgeTokenInfoRes
	err := rpcClient.RPCCall("getallbridgetokens", params, &bridgeTokenInfoRes)
	if err != nil {
		return 0, err
	}
	if bridgeTokenInfoRes.RPCError != nil {
		logger.Errorf("getallbridgetokens: call RPC error, %v\n", bridgeTokenInfoRes.RPCError.StackTrace)
		return 0, errors.New(bridgeTokenInfoRes.RPCError.Message)
	}

	for _, token := range bridgeTokenInfoRes.Result {
		if token.TokenID == tokenID {
			return token.Amount, nil
		}
	}

	logger.Errorf("Can not found tokenID %v\n", tokenID)
	return 0, fmt.Errorf("Can not found tokenID %v\n", tokenID)
}

func getIncognitoBlockChainInfo(
	rpcClient *utils.HttpClient, logger *logrus.Entry,
) (*entities.GetBlockChainInfoResult,
	error) {
	params := []interface{}{
		map[string]string{},
	}
	var blockChainInfo entities.GetBlockChainInfoRes
	err := rpcClient.RPCCall("getblockchaininfo", params, &blockChainInfo)
	if err != nil {
		return nil, err
	}
	if blockChainInfo.RPCError != nil {
		logger.Errorf("getblockchaininfo: call RPC error, %v\n", blockChainInfo.RPCError.StackTrace)
		return nil, errors.New(blockChainInfo.RPCError.Message)
	}
	return blockChainInfo.Result, nil
}

func getRequestUnShieldingStatus(rpcClient *utils.HttpClient, txID string) (*entities.UnShieldingRequestStatus, error) {
	params := []interface{}{
		map[string]string{
			"UnshieldID": txID,
		},
	}

	var unshieldReqStatusRes entities.UnShieldingRequestStatusRes
	err := rpcClient.RPCCall("getportalunshieldrequeststatus", params, &unshieldReqStatusRes)
	if err != nil {
		return nil, err
	}
	if unshieldReqStatusRes.RPCError != nil {
		return unshieldReqStatusRes.Result, errors.New(unshieldReqStatusRes.RPCError.Message)
	}
	return unshieldReqStatusRes.Result, nil
}


func ConvertBTCExternalToIncAmount(externalAmt uint64) uint64 {
	return externalAmt * 10
}

func ConvertBTCIncToExternalAmount(incAmt uint64) uint64 {
	return incAmt / 10 // incAmt / 10^9 * 10^8
}

