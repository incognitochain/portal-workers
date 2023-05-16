package workers

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"fmt"
	"os"

	"github.com/incognitochain/go-incognito-sdk-v2/common"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"

	resty "github.com/go-resty/resty/v2"
)

type BlockchainFeeResponse struct {
	Result float64
	Error  error
}

func getBitcoinFee() (float64, error) {
	client := resty.New()

	response, err := client.R().
		Get(os.Getenv("BLOCKCHAIN_FEE_HOST"))

	if err != nil {
		return 0, err
	}
	if response.StatusCode() != 200 {
		return 0, fmt.Errorf("Response status code: %v", response.StatusCode())
	}
	var responseBody BlockchainFeeResponse
	err = json.Unmarshal(response.Body(), &responseBody)
	if err != nil {
		return 0, fmt.Errorf("Could not parse response: %v", response.Body())
	}
	return responseBody.Result, nil
}

func broadcastTxToBlockStream(txHex string) (string, error) {
	client := resty.New()

	url := "https://blockstream.info/api/tx"

	response, err := client.R().SetBody(txHex).Post(url)
	if err != nil {
		return "", err
	}
	fmt.Printf("response: %+v\n", response)
	if response.StatusCode() != 200 {
		return "", fmt.Errorf("Request status code: %v - Error %v", response.StatusCode(), response)
	}

	return response.String(), nil
}

func GetListShieldingAddress(from int64, to int64) ([]*ShieldingMonitoringInfo, error) {
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

func GetPortalState(
	height uint64, rpcClient *utils.HttpClient,
) (*entities.PortalV4State, error) {
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
		// logger.Errorf("getportalv4state: call RPC error, %v\n", portalStateRes.RPCError.StackTrace)
		return nil, errors.New(portalStateRes.RPCError.Message)
	}
	return portalStateRes.Result, nil
}

// Prefix length
const (
	prefixHashKeyLength = 12
	prefixKeyLength     = 20
)

func BytesToHash(b []byte) common.Hash {
	var h common.Hash
	_ = h.SetBytes(b)
	//if err != nil {
	//	panic(err)
	//}
	return h
}

func GenerateUTXOObjectKey(tokenID string, walletAddress string, txHash string, outputIdx uint32) common.Hash {
	prefixHash := GetPortalUTXOStatePrefix(tokenID)
	paddedWalletAddress := walletAddress
	if len(walletAddress) < 40 {
		paddedWalletAddress = strings.Repeat("0", 40-len(walletAddress)) + walletAddress
	}
	paddedOutputIdx := fmt.Sprintf("%05d", outputIdx)

	value := append([]byte(paddedWalletAddress), []byte(txHash)...)
	value = append(value, []byte(paddedOutputIdx)...)
	valueHash := common.HashH(value)
	return BytesToHash(append(prefixHash, valueHash[:][:prefixKeyLength]...))
}

func GetPortalUTXOStatePrefix(tokenID string) []byte {
	h := common.HashH(append([]byte("portalutxo-"), []byte(tokenID)...))
	return h[:][:prefixHashKeyLength]
}
