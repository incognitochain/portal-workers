package workers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	resty "github.com/go-resty/resty/v2"
	"github.com/incognitochain/go-incognito-sdk-v2/coin"
	"github.com/incognitochain/go-incognito-sdk-v2/common"
	"github.com/incognitochain/portal-workers/entities"
)

type PortalAddressInstance struct {
	IncAddress string `json:"incaddress"`
	BTCAddress string `json:"btcaddress"`
	TimeStamp  int64  `json:"timestamp"`
}

type PortalBackendRes struct {
	Result []*PortalAddressInstance
	Error  interface{}
}

type AirdropRequest struct {
	PaymentAddress string `json:"paymentaddress"`
}

type ErrorInfo struct {
	Code int    `json:"Code"`
	Msg  string `json:"Msg"`
}

type AirdropResponse struct {
	Result int `json:"Result"`
}

func (b *BTCWalletMonitor) submitShieldingRequest(incAddress string, proof string) (string, error) {
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

func (b *BTCWalletMonitor) getRequestShieldingStatus(txID string) (int, string, error) {
	params := []interface{}{
		map[string]string{
			"ReqTxID": txID,
		},
	}

	var requestShieldingStatusRes entities.ShieldingRequestStatusRes

	var err error
	for idx := 0; idx < NumGetStatusTries; idx++ {
		time.Sleep(IntervalTries)
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

func (b *BTCWalletMonitor) getTrackingInstance(from int64, to int64) ([]*ShieldingMonitoringInfo, error) {
	list := []*ShieldingMonitoringInfo{}

	apiURL := fmt.Sprintf("%v/getlistportalshieldingaddress?from=%v&to=%v", os.Getenv("BACKEND_API_HOST"), from, to)
	response, err := http.Get(apiURL)
	if err != nil {
		return list, err
	}
	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return list, err
	}
	var responseBody PortalBackendRes
	err = json.Unmarshal(responseData, &responseBody)
	if err != nil {
		return list, err
	}
	if responseBody.Error != nil {
		return list, fmt.Errorf(responseBody.Error.(string))
	}

	for _, instance := range responseBody.Result {
		list = append(list, &ShieldingMonitoringInfo{
			IncAddress:  instance.IncAddress,
			BTCAddress:  instance.BTCAddress,
			TimeStamp:   instance.TimeStamp,
			ScannedTxID: map[string]int64{},
		})
	}

	return list, nil
}

// hashProof returns the hash of shielding proof (include tx proof and user inc address)
func hashProof(proof string, incAddressStr string) string {
	type shieldingProof struct {
		Proof      string
		IncAddress string
	}

	shieldProof := shieldingProof{
		Proof:      proof,
		IncAddress: incAddressStr,
	}
	shieldProofBytes, _ := json.Marshal(shieldProof)
	hash := common.HashB(shieldProofBytes)
	return fmt.Sprintf("%x", hash[:])
}

func convertBTCtoNanopBTC(amount float64) uint64 {
	return uint64(amount*1e9 + 0.5)
}

func sendAirdropRequest(incAddress string) (int, error) {
	client := resty.New()

	response, err := client.R().
		SetBody(AirdropRequest{PaymentAddress: incAddress}).
		Post(os.Getenv("AIRDROP_PRV_HOST"))

	if err != nil {
		return 0, err
	}
	if response.StatusCode() != 200 {
		return 0, fmt.Errorf("Response status code: %v", response.StatusCode())
	}
	var responseBody AirdropResponse
	err = json.Unmarshal(response.Body(), &responseBody)
	if err != nil {
		return 0, fmt.Errorf("Could not parse response: %v", response.Body())
	}
	return responseBody.Result, nil
}
