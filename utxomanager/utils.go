package utxomanager

import (
	"errors"

	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
)

func GetTxByHash(rpcClient *utils.HttpClient, txID string) (*entities.TxDetail, error) {
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
