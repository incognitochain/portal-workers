package transaction

import (
	"fmt"

	"github.com/inc-backend/go-incognito/common"
	"github.com/inc-backend/go-incognito/src/createrawdata"
	"github.com/inc-backend/go-incognito/src/httpclient"
	"github.com/inc-backend/go-incognito/src/implement"
	"github.com/inc-backend/go-incognito/src/services"
	metadata "github.com/incognitochain/portal-workers/metadatav2"
)

type ShieldingRequest implement.Transaction

func (a *ShieldingRequest) BuildRawData() (interface{}, error) {
	arrayParams := common.InterfaceSlice(a.Params)
	if arrayParams == nil || len(arrayParams) < 5 {
		return nil, fmt.Errorf("param must be an array at least 5 element")
	}

	// get meta data from params
	data, ok := arrayParams[4].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("metadata param is invalid")
	}
	tokenID, ok := data["TokenID"].(string)
	if !ok {
		return nil, fmt.Errorf("metadata TokenID is invalid")
	}
	incognitoAddress, ok := data["IncogAddressStr"].(string)
	if !ok {
		return nil, fmt.Errorf("metadata IncogAddressStr is invalid")
	}

	shieldingProof, ok := data["ShieldingProof"].(string)
	if !ok {
		return nil, fmt.Errorf("metadata ShieldingProof param is invalid")
	}

	meta, err := metadata.NewPortalShieldingRequest(
		PortalShieldingRequestMeta,
		tokenID,
		incognitoAddress,
		shieldingProof,
	)

	if err != nil {
		return nil, err
	}

	txService := services.NewTxService(a.RpcClient, a.Version)
	createRawTxParam, errNewParam := createrawdata.NewCreateRawTxParam(a.Params)
	if errNewParam != nil {
		return nil, errNewParam
	}
	txHash, txType, txBytes, txShardID, err := txService.CreateRawTransaction(createRawTxParam, meta)
	if err != nil {
		return nil, err
	}

	result := httpclient.NewCreateTransactionResult(txHash, txType, txBytes, txShardID)
	return result, nil
}
