package workers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/inc-backend/go-incognito/common"
	"github.com/inc-backend/go-incognito/publish/transformer"
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

func (b *BTCWalletMonitor) submitShieldingRequest(incAddress string, proof string) (string, error) {
	metadata := map[string]interface{}{
		"IncogAddressStr": incAddress,
		"TokenID":         BTCID,
		"ShieldingProof":  proof,
	}

	utxos, tmpTxID, err := b.UTXOManager.GetUTXOsByAmount(os.Getenv("INCOGNITO_PRIVATE_KEY"), 5000)
	if err != nil {
		return "", err
	}
	utxoKeyImages := []string{}
	for _, utxo := range utxos {
		utxoKeyImages = append(utxoKeyImages, utxo.KeyImage)
	}

	result, err := b.Portal.Shielding(os.Getenv("INCOGNITO_PRIVATE_KEY"), metadata, utxoKeyImages)
	if err != nil {
		b.UTXOManager.UncachedUTXOByTmpTxID(os.Getenv("INCOGNITO_PRIVATE_KEY"), tmpTxID)
		return "", err
	}
	resp, err := b.Client.SubmitRawData(result)
	if err != nil {
		b.UTXOManager.UncachedUTXOByTmpTxID(os.Getenv("INCOGNITO_PRIVATE_KEY"), tmpTxID)
		return "", err
	}

	txID, err := transformer.TransformersTxHash(resp)
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
