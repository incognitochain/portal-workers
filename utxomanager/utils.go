package utxomanager

import (
	"errors"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
)

func getTxByHash(rpcClient *utils.HttpClient, txID string) (*entities.TxDetail, error) {
	var txByHashRes entities.TxDetailRes
	params := []interface{}{
		txID,
	}
	err := rpcClient.RPCCall("gettransactionbyhash", params, &txByHashRes)
	if err != nil {
		return nil, err
	}
	if txByHashRes.RPCError != nil {
		return nil, errors.New(txByHashRes.RPCError.Message)
	}
	return txByHashRes.Result, nil
}

func getListUTXOs(w *go_incognito.Wallet, privateKey string) ([]UTXO, error) {
	inputCoins, err := w.GetUTXO(privateKey, PRVIDStr)
	if err != nil {
		return []UTXO{}, err
	}

	utxos := []UTXO{}
	for _, coin := range inputCoins {
		utxos = append(utxos, UTXO{
			KeyImage: coin.KeyImages,
			Amount:   coin.Value,
		})
	}
	return utxos, nil
}
