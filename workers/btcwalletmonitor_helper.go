package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/inc-backend/go-incognito/common"
	"github.com/inc-backend/go-incognito/publish/transformer"
	"github.com/incognitochain/portal-workers/entities"
)

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
	// TODO
	return []*ShieldingMonitoringInfo{}, nil
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
