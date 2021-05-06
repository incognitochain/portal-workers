package workers

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/inc-backend/go-incognito/publish/transformer"
	"github.com/incognitochain/portal-workers/entities"
)

func (b *BTCWalletMonitor) submitShieldingRequest(incAddress string, proof string) (string, error) {
	metadata := map[string]interface{}{
		"IncogAddressStr": incAddress,
		"TokenID":         BTCID,
		"ShieldingProof":  proof,
	}

	result, err := b.Portal.Shielding(os.Getenv("INCOGNITO_PRIVATE_KEY"), metadata)
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

func (b *BTCWalletMonitor) getRequestShieldingStatus(txID string) (int, error) {
	params := []interface{}{
		map[string]string{
			"ReqTxID": txID,
		},
	}

	var requestShieldingStatusRes entities.RequestStatusRes

	var err error
	for idx := 0; idx < NUM_GET_STATUS_TRIES; idx++ {
		time.Sleep(INTERVAL_TRIES)
		err = b.RPCClient.RPCCall("getportalshieldingrequeststatus", params, &requestShieldingStatusRes)
		if err == nil && requestShieldingStatusRes.RPCError == nil {
			return requestShieldingStatusRes.Result.Status, nil
		}
	}

	if err != nil {
		return 0, err
	} else {
		return 0, fmt.Errorf(requestShieldingStatusRes.RPCError.Message)
	}
}

func (b *BTCWalletMonitor) getLatestBTCBlockHashFromIncog() (uint64, error) {
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

func (b *BTCWalletMonitor) getTrackingInstance(from int64, to int64) ([]*ShieldingMonitoringInfo, error) {
	// TODO
	return []*ShieldingMonitoringInfo{}, nil
}
