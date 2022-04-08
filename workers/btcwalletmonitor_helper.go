package workers

import (
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/incognitochain/go-incognito-sdk-v2/incclient"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/syndtr/goleveldb/leveldb"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

	resty "github.com/go-resty/resty/v2"
	"github.com/incognitochain/go-incognito-sdk-v2/coin"
	"github.com/incognitochain/go-incognito-sdk-v2/common"
	"github.com/incognitochain/portal-workers/entities"
)

// PortalShieldingInstance is a record representing a shielding request.
type PortalShieldingInstance struct {
	// IncAddress is an Incognito payment address associated with the shielding request. It is used in the
	// old shielding procedure in which BTCAddress is repeated for every shielding request of the same IncAddress.
	IncAddress string `json:"incaddress,omitempty"`

	// OTDepositPubKey is a one-time depositing public key associated with the shielding request. It is used to replace
	// the old shielding procedure and provides better privacy level.
	// Either IncAddress or OTDepositPubKey must be non-empty.
	OTDepositPubKey string `json:"depositkey,omitempty"`

	// Receivers is a list of OTAReceivers for receiving the shielding assets.
	// It is only used with OTDepositPubKey.
	Receivers []string `json:"receivers,omitempty"`

	// Signatures is a list of valid signatures signed on each OTAReceiver against the OTDepositPubKey.
	// It is only used with OTDepositPubKey.
	Signatures []string `json:"signatures,omitempty"`

	// BTCAddress is the multi-sig address for receiving public token. It is generated based on either IncAddress or
	// OTDepositPubKey.
	BTCAddress string `json:"btcaddress"`

	// TimeStamp is the initializing time of the request.
	TimeStamp int64 `json:"timestamp"`
}

type PortalBackendResp struct {
	Result []*PortalShieldingInstance
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
	Error  *ErrorInfo `json:"Error"`
	Result string     `json:"Result"`
}

func (b *BTCWalletMonitor) loadData() (*ShieldingInfoManager, error) {
	b.ExportErrorLog("BTCWalletMonitor worker is loading data...")
	var err error
	b.db, err = leveldb.OpenFile(WalletMonitorDBFileDir, nil)
	if err != nil {
		err = fmt.Errorf("could not open leveldb storage file - with err: %v", err)
		return nil, err
	}
	defer b.db.Close()

	shieldingInfoManager := &ShieldingInfoManager{
		MonitoringList:            []*ShieldingMonitoringInfo{},
		WaitingList:               map[string]*ShieldingRequestInfo{},
		LastTimeUpdate:            int64(0),
		LastScannedBTCBlockHeight: int64(FirstScannedBTCBlkHeight - 1),
	}
	// restore data from db
	lastUpdateBytes, err := b.db.Get([]byte(WalletMonitorDBObjectName), nil)
	if err == nil {
		var tmpShieldingInfoManager *ShieldingInfoManager
		err = json.Unmarshal(lastUpdateBytes, &tmpShieldingInfoManager)
		if err == nil {
			tmpShieldingInfoManager = shieldingInfoManager
		}
	}

	return shieldingInfoManager, nil
}

func (b *BTCWalletMonitor) saveData(m *ShieldingInfoManager) error {
	shieldingTxArrayObjectBytes, _ := json.Marshal(m)
	err := b.db.Put([]byte(WalletMonitorDBObjectName), shieldingTxArrayObjectBytes, nil)
	if err != nil {
		return err
	}

	return nil
}

func (b *BTCWalletMonitor) buildShieldingRequestInfo(monitorInfo *ShieldingMonitoringInfo,
	txHash string,
	chainCode string,
	wg *sync.WaitGroup,
	shieldingRequestChan chan map[string]*ShieldingRequestInfo,
) {
	defer wg.Done()

	txID, _ := chainhash.NewHashFromStr(txHash)
	tx, _ := b.btcClient.GetRawTransactionVerbose(txID)
	blkHash, _ := chainhash.NewHashFromStr(tx.BlockHash)
	blk, err := b.btcClient.GetBlockHeaderVerbose(blkHash)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get block height of block hash: %v for txID: %v - with err: %v", blkHash, txID, err))
		return
	}

	// generate proof
	proof, err := utils.BuildProof(b.btcClient, txHash, uint64(blk.Height))
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not build proof for tx: %v - with err: %v", txHash, err))
		return
	}

	fmt.Printf("Found shielding request for address %v, with BTC tx %v\n", chainCode, txHash)
	proofHash := hashProof(proof, chainCode)
	shieldingRequestChan <- map[string]*ShieldingRequestInfo{
		proofHash: {
			TxHash: txHash,
			ShieldingData: ShieldingData{
				IncAddress:      monitorInfo.IncAddress,
				OTDepositPubKey: monitorInfo.OTDepositPubKey,
				Receivers:       monitorInfo.Receivers,
				Signatures:      monitorInfo.Signatures,
			},
			Proof:          proof,
			BTCBlockHeight: uint64(blk.Height),
		},
	}
}

func (b *BTCWalletMonitor) shield(proofHash string,
	reqInfo *ShieldingRequestInfo,
	shardID int,
	wg *sync.WaitGroup,
	shieldingTxChan chan string,
) {
	defer wg.Done()

	curProofHash := proofHash
	curTxHash := reqInfo.TxHash

	// create shielding tx
	txID, err := b.submitShieldingRequest(reqInfo)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not send shielding request from BTC tx %v proof with err: %v", curTxHash, err))
		return
	}
	fmt.Printf("Shielding txID: %v\n", txID)
	status, errorStr, err := b.getRequestShieldingStatus(txID)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get request shielding status from BTC tx %v - with err: %v", curTxHash, err))
	} else {
		ok := isFinalizedTx(b.UTXOManager.IncClient, b.Logger, shardID, txID)
		if !ok {
			return
		}
		if status == 0 { // rejected
			if errorStr == "IsExistedProof" {
				b.ExportErrorLog(fmt.Sprintf("Request shielding failed BTC tx %v, shielding txID %v - duplicated proof", curTxHash, txID))
				shieldingTxChan <- curProofHash
			} else {
				b.ExportErrorLog(fmt.Sprintf("Request shielding failed BTC tx %v, shielding txID %v - invalid proof", curTxHash, txID))
			}
		} else if reqInfo.IncAddress != "" {
			airdropTxID, err := sendAirdropRequest(reqInfo.IncAddress)
			if err == nil {
				b.ExportInfoLog(fmt.Sprintf("Request shielding succeed BTC tx %v, shielding txID %v, airdrop TxID: %v", curTxHash, txID, airdropTxID))
			} else {
				b.ExportInfoLog(fmt.Sprintf("Request shielding succeed BTC tx %v, shielding txID %v, airdrop failed - %v", curTxHash, txID, err))
			}

			shieldingTxChan <- curProofHash
		}

	}
}

// submitShieldingRequest creates and submits a shielding transaction on behalf of the user.
func (b *BTCWalletMonitor) submitShieldingRequest(shieldingInfo *ShieldingRequestInfo) (string, error) {
	if shieldingInfo.IncAddress != "" {
		return b.submitShieldingRequestWithIncAddress(shieldingInfo)
	}

	return b.submitShieldingRequestWithDepositKey(shieldingInfo)
}

// submitShieldingRequestWithIncAddress creates and submits a shielding transaction on behalf of the user using
// his/her Incognito address.
func (b *BTCWalletMonitor) submitShieldingRequestWithIncAddress(shieldingInfo *ShieldingRequestInfo) (string, error) {
	utxos, tmpTxID, err := b.UTXOManager.GetUTXOsByAmount(os.Getenv("INCOGNITO_PRIVATE_KEY"), 5000)
	if err != nil {
		return "", err
	}
	utxoCoins := make([]coin.PlainCoin, 0)
	utxoIndices := make([]uint64, 0)
	for _, utxo := range utxos {
		utxoCoins = append(utxoCoins, utxo.Coin)
		utxoIndices = append(utxoIndices, utxo.Index.Uint64())
	}

	txID, err := b.UTXOManager.IncClient.CreateAndSendPortalShieldTransaction(
		os.Getenv("INCOGNITO_PRIVATE_KEY"),
		BTCID,
		shieldingInfo.IncAddress,
		shieldingInfo.Proof,
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

// submitShieldingRequestWithDepositKey creates and submits a shielding transaction on behalf of the user using
// his/her deposit key.
func (b *BTCWalletMonitor) submitShieldingRequestWithDepositKey(shieldingInfo *ShieldingRequestInfo) (string, error) {
	utxos, tmpTxID, err := b.UTXOManager.GetUTXOsByAmount(os.Getenv("INCOGNITO_PRIVATE_KEY"), 5000)
	if err != nil {
		return "", err
	}
	utxoCoins := make([]coin.PlainCoin, 0)
	utxoIndices := make([]uint64, 0)
	for _, utxo := range utxos {
		utxoCoins = append(utxoCoins, utxo.Coin)
		utxoIndices = append(utxoIndices, utxo.Index.Uint64())
	}

	var receiver string
	var signature string
	var txID string
	// Get receiver and signature
	for i, receiverStr := range shieldingInfo.Receivers {
		if exists, err := b.UTXOManager.IncClient.HasOTAReceiver(receiverStr); exists || err != nil {
			continue
		}
		receiver = receiverStr
		signature = shieldingInfo.Signatures[i]
	}

	if receiver == "" { // when no possible receiver exists
		err = fmt.Errorf("all receivers have been used")
	} else {
		depositParams := incclient.PortalDepositParams{
			TokenID:       BTCID,
			ShieldProof:   shieldingInfo.Proof,
			DepositPubKey: shieldingInfo.OTDepositPubKey,
			Receiver:      receiver,
			Signature:     signature,
		}

		txID, err = b.UTXOManager.IncClient.CreateAndSendPortalShieldTransactionWithDepositKey(
			os.Getenv("INCOGNITO_PRIVATE_KEY"),
			depositParams,
			utxoCoins,
			utxoIndices,
		)
	}
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
	list := make([]*ShieldingMonitoringInfo, 0)

	apiURL := fmt.Sprintf("%v/getlistportalshieldingaddress?from=%v&to=%v", os.Getenv("BACKEND_API_HOST"), from, to)
	response, err := http.Get(apiURL)
	if err != nil {
		return list, err
	}
	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return list, err
	}
	var responseBody PortalBackendResp
	err = json.Unmarshal(responseData, &responseBody)
	if err != nil {
		return list, err
	}
	if responseBody.Error != nil {
		return list, fmt.Errorf(responseBody.Error.(string))
	}

	for _, instance := range responseBody.Result {
		list = append(list, &ShieldingMonitoringInfo{
			ShieldingData: ShieldingData{
				IncAddress:      instance.IncAddress,
				OTDepositPubKey: instance.OTDepositPubKey,
				Receivers:       instance.Receivers,
				Signatures:      instance.Signatures,
			},
			BTCAddress:  instance.BTCAddress,
			TimeStamp:   instance.TimeStamp,
			ScannedTxID: map[string]int64{},
		})
	}

	return list, nil
}

// hashProof returns the hash of shielding proof (include tx proof and user inc address)
func hashProof(proof string, chainCode string) string {
	type shieldingProof struct {
		Proof      string
		IncAddress string
	}

	shieldProof := shieldingProof{
		Proof:      proof,
		IncAddress: chainCode,
	}
	shieldProofBytes, _ := json.Marshal(shieldProof)
	hash := common.HashB(shieldProofBytes)
	return fmt.Sprintf("%x", hash[:])
}

func convertBTCtoNanopBTC(amount float64) uint64 {
	return uint64(amount*1e9 + 0.5)
}

func sendAirdropRequest(incAddress string) (string, error) {
	client := resty.New()

	response, err := client.R().
		SetBody(AirdropRequest{PaymentAddress: incAddress}).
		Post(os.Getenv("AIRDROP_PRV_HOST"))

	if err != nil {
		return "", err
	}
	if response.StatusCode() != 200 {
		return "", fmt.Errorf("Response status code: %v", response.StatusCode())
	}
	var responseBody AirdropResponse
	err = json.Unmarshal(response.Body(), &responseBody)
	if err != nil {
		return "", fmt.Errorf("Could not parse response: %v", response.Body())
	}
	if responseBody.Error != nil {
		return "", fmt.Errorf("Error: %v", responseBody.Error)
	}
	return responseBody.Result, nil
}
