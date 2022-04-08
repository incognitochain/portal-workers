package workers

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcutil"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	FirstScannedBTCBlkHeight        = 697200
	TimeoutTrackingInstanceInSecond = int64(365 * 24 * 60 * 60)
	WalletMonitorDBFileDir          = "db/walletmonitor"
	WalletMonitorDBObjectName       = "BTCMonitor-LastUpdate"
	MinShieldAmount                 = 100000 // nano pBTC
)

type BTCWalletMonitor struct {
	WorkerAbs
	btcClient *rpcclient.Client
	chainCfg  *chaincfg.Params
	db        *leveldb.DB
}

type ShieldingData struct {
	// IncAddress is an Incognito payment address associated with the shielding request. It is used in the
	// old shielding procedure in which BTCAddress is repeated for every shielding request of the same IncAddress.
	IncAddress string `json:"IncAddress,omitempty"`

	// OTDepositPubKey is a one-time depositing public key associated with the shielding request. It is used to replace
	// the old shielding procedure and provides better privacy level.
	// Either IncAddress or OTDepositPubKey must be non-empty.
	OTDepositPubKey string `json:"OTDepositPubKey,omitempty"`

	// Receivers is a list of OTAReceivers for receiving the shielding assets.
	// It is only used with OTDepositPubKey.
	Receivers []string `json:"Receivers,omitempty"`

	// Signatures is a list of valid signatures signed on each OTAReceiver against the OTDepositPubKey.
	// It is only used with OTDepositPubKey.
	Signatures []string `json:"Signatures,omitempty"`
}

type ShieldingMonitoringInfo struct {
	ShieldingData

	// BTCAddress is the multi-sig address for receiving public token. It is generated based on either IncAddress or
	// OTDepositPubKey.
	BTCAddress string `json:"BTCAddress"`

	// TimeStamp is the initializing time of the request.
	TimeStamp int64 `json:"TimeStamp"`

	// ScannedTxID mapping (from txHash -> timeStamp) associated with the BTCAddress.
	ScannedTxID map[string]int64
}

type ShieldingRequestInfo struct {
	ShieldingData
	TxHash         string `json:"TxHash"`
	Proof          string `json:"Proof"`
	BTCBlockHeight uint64 `json:"BTCBlockHeight"`
}

type ShieldingInfoManager struct {
	MonitoringList            []*ShieldingMonitoringInfo
	WaitingList               map[string]*ShieldingRequestInfo // key: proofHash
	LastTimeUpdate            int64
	LastScannedBTCBlockHeight int64
}

func (b *BTCWalletMonitor) Init(id int, name string, freq int, network string, utxoManager *utxomanager.UTXOManager) error {
	b.WorkerAbs.Init(id, name, freq, network, utxoManager)

	var err error

	// init bitcoin rpcclient
	b.btcClient, err = utils.BuildBTCClient()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not initialize Bitcoin RPCClient - with err: %v", err))
		return err
	}

	b.chainCfg = &chaincfg.TestNet3Params
	if b.Network == "main" {
		b.chainCfg = &chaincfg.MainNetParams
	} else if b.Network == "reg" {
		b.chainCfg = &chaincfg.RegressionNetParams
	}

	return nil
}

func (b *BTCWalletMonitor) ExportErrorLog(msg string) {
	b.WorkerAbs.ExportErrorLog(msg)
}

func (b *BTCWalletMonitor) ExportInfoLog(msg string) {
	b.WorkerAbs.ExportInfoLog(msg)
}

// Execute has 2 main tasks:
// - Monitor Bitcoin multi-sig wallets that corresponding with Incognito Wallet App users.
// - Send shielding request on behalf of users to Incognito chain.
func (b *BTCWalletMonitor) Execute() {
	b.ExportErrorLog("BTCWalletMonitor worker is executing...")

	// load the current state
	m, err := b.loadData()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return
	}

	trackingBTCAddresses := []btcutil.Address{}
	btcAddressIndexMapping := map[string]int{}
	for idx, instance := range m.MonitoringList {
		address, err := btcutil.DecodeAddress(instance.BTCAddress, b.chainCfg)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not decode address %v - with err: %v", instance.BTCAddress, err))
			continue
		}
		trackingBTCAddresses = append(trackingBTCAddresses, address)
		btcAddressIndexMapping[instance.BTCAddress] = idx
	}

	shardID, _ := strconv.Atoi(os.Getenv("SHARD_ID"))

	for {
		// check BTC node alive
		isBTCNodeAlive := getBTCFullnodeStatus(b.btcClient)
		if !isBTCNodeAlive {
			b.ExportErrorLog("Could not connect to BTC full node")
			return
		}

		// get new rescanning instance from API
		currentTimeStamp := time.Now().Unix()
		newlyTrackingInstance, err := b.getTrackingInstance(m.LastTimeUpdate, currentTimeStamp)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get tracking instance from API - with err: %v", err))
			return
		}
		m.LastTimeUpdate = currentTimeStamp

		// delete timeout tracking scanned txID
		for _, instance := range m.MonitoringList {
			for txID, timestamp := range instance.ScannedTxID {
				if timestamp+TimeoutTrackingInstanceInSecond < currentTimeStamp {
					delete(instance.ScannedTxID, txID)
				}
			}
		}

		for _, instance := range newlyTrackingInstance {
			_, exists := btcAddressIndexMapping[instance.BTCAddress]
			if exists {
				continue
			}

			address, err := btcutil.DecodeAddress(instance.BTCAddress, b.chainCfg)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not decode address %v - with err: %v", instance.BTCAddress, err))
				continue
			}

			m.MonitoringList = append(m.MonitoringList, instance)
			trackingBTCAddresses = append(trackingBTCAddresses, address)
			btcAddressIndexMapping[instance.BTCAddress] = len(m.MonitoringList) - 1
		}
		if len(trackingBTCAddresses) == 0 {
			time.Sleep(15 * time.Second)
			continue
		}

		btcBestBlockHeight, err := b.btcClient.GetBlockCount()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get BTC best block height - with err: %v", err))
			return
		}
		minConfirmation := BTCConfirmationThreshold
		maxConfirmation := int(btcBestBlockHeight - m.LastScannedBTCBlockHeight + 30) // Over confirmation just for sure
		if maxConfirmation < minConfirmation {
			maxConfirmation = minConfirmation
		}
		listUnspentResults, err := b.btcClient.ListUnspentMinMaxAddresses(minConfirmation, maxConfirmation, trackingBTCAddresses)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not scan list of unspent outcoins - with err: %v", err))
			return
		}
		m.LastScannedBTCBlockHeight = btcBestBlockHeight - int64(minConfirmation) + 1

		// get shielding request list
		var wg sync.WaitGroup
		shieldingRequestChan := make(chan map[string]*ShieldingRequestInfo, len(listUnspentResults))
		for _, unspentCoins := range listUnspentResults {
			btcAddress := unspentCoins.Address
			idx := btcAddressIndexMapping[btcAddress]
			chainCode := m.MonitoringList[idx].IncAddress
			if chainCode == "" {
				chainCode = m.MonitoringList[idx].OTDepositPubKey
			}

			txHash := unspentCoins.TxID
			_, exists := m.MonitoringList[idx].ScannedTxID[txHash]
			if exists || convertBTCtoNanopBTC(unspentCoins.Amount) < MinShieldAmount {
				continue
			}
			m.MonitoringList[idx].ScannedTxID[txHash] = currentTimeStamp

			wg.Add(1)
			go b.buildShieldingRequestInfo(m.MonitoringList[idx], txHash, chainCode, &wg, shieldingRequestChan)
		}
		wg.Wait()
		close(shieldingRequestChan)
		for shieldRequest := range shieldingRequestChan {
			for key, value := range shieldRequest {
				m.WaitingList[key] = value
			}
		}

		// send shielding request transactions to Incognito chain
		relayingBTCHeight, err := getLatestBTCHeightFromIncog(b.RPCBTCRelayingReaders)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}
		fmt.Printf("Number of waiting shielding requests: %v\n", len(m.WaitingList))
		fmt.Printf("Number of shielding monitoring instances: %v\n", len(m.MonitoringList))
		shieldingTxChan := make(chan string, len(m.WaitingList))
		for proofHash, value := range m.WaitingList {
			if value.BTCBlockHeight+BTCConfirmationThreshold-1 <= relayingBTCHeight {
				wg.Add(1)
				go b.shield(proofHash, value, shardID, &wg, shieldingTxChan)
			}
		}
		wg.Wait()
		close(shieldingTxChan)
		for proofHash := range shieldingTxChan {
			delete(m.WaitingList, proofHash)
		}

		// save the state
		err = b.saveData(m)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}
		time.Sleep(100 * time.Second)
	}
}
