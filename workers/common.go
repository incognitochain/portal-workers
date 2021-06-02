package workers

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/rpcclient"
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

func getFinalizedBeaconHeight(rpcClient *utils.HttpClient, logger *logrus.Entry) (uint64, error) {
	params := []interface{}{
		-1,
	}
	var allViewRes entities.AllViewRes
	err := rpcClient.RPCCall("getallviewdetail", params, &allViewRes)
	if err != nil {
		return 0, err
	}

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

func getBatchIDsFromBeaconHeight(height uint64, rpcClient *utils.HttpClient, logger *logrus.Entry) ([]string, error) {
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
		batchIDs = append(batchIDs, batch.BatchID)
	}
	return batchIDs, nil
}
