package workers

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcutil"
	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/syndtr/goleveldb/leveldb"
)

const TimeoutTrackingInstanceInSecond = int64(365 * 24 * 60 * 60)

type BTCWalletMonitor struct {
	WorkerAbs
	Portal    *go_incognito.Portal
	btcClient *rpcclient.Client
	chainCfg  *chaincfg.Params
	db        *leveldb.DB
}

type ShieldingMonitoringInfo struct {
	IncAddress  string
	BTCAddress  string
	TimeStamp   int64
	ScannedTxID map[string]int64
}

type ShieldingRequestInfo struct {
	IncAddress     string
	Proof          string
	BTCBlockHeight uint64
}

type ShieldingTxArrayObject struct {
	ShieldingMonitoringList []*ShieldingMonitoringInfo
	WaitingShieldingList    map[string]*ShieldingRequestInfo // key: txHash
	LastTimeStampUpdated    int64
}

func (b *BTCWalletMonitor) Init(id int, name string, freq int, network string) error {
	b.WorkerAbs.Init(id, name, freq, network)

	b.Portal = go_incognito.NewPortal(b.Client)

	var err error
	// init leveldb instance
	b.db, err = leveldb.OpenFile("db/walletmonitor", nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return err
	}

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

// This function will execute a worker that has 2 main tasks:
// - Monitor Bitcoin multisig wallets that corresponding with Incognito Wallet App users
// - Send shielding request on behalf of users to Incognito chain
func (b *BTCWalletMonitor) Execute() {
	b.Logger.Info("BTCWalletMonitor worker is executing...")
	defer b.db.Close()

	shieldingMonitoringList := []*ShieldingMonitoringInfo{}
	waitingShieldingList := map[string]*ShieldingRequestInfo{}
	lastTimeUpdated := int64(0)

	// restore data from db
	lastUpdateBytes, err := b.db.Get([]byte("BTCMonitor-LastUpdate"), nil)
	if err == nil {
		var shieldingTxArrayObject *ShieldingTxArrayObject
		json.Unmarshal(lastUpdateBytes, &shieldingTxArrayObject)
		shieldingMonitoringList = shieldingTxArrayObject.ShieldingMonitoringList
		waitingShieldingList = shieldingTxArrayObject.WaitingShieldingList
		lastTimeUpdated = shieldingTxArrayObject.LastTimeStampUpdated
	}

	trackingBTCAddress := []btcutil.Address{}
	btcAddressIndexMapping := map[string]int{}
	for idx, instance := range shieldingMonitoringList {
		address, err := btcutil.DecodeAddress(instance.BTCAddress, b.chainCfg)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not decode address %v - with err: %v", instance.BTCAddress, err))
			continue
		}
		trackingBTCAddress = append(trackingBTCAddress, address)
		btcAddressIndexMapping[instance.BTCAddress] = idx
	}

	for {
		// get new rescanning instance from API
		currentTimeStamp := time.Now().Unix()
		newlyTrackingInstance, err := b.getTrackingInstance(lastTimeUpdated, currentTimeStamp)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get tracking instance from API - with err: %v", err))
			return
		}
		lastTimeUpdated = currentTimeStamp

		// delete timeout tracking scanned txID
		for _, instance := range shieldingMonitoringList {
			for txID, timestamp := range instance.ScannedTxID {
				if timestamp+TimeoutTrackingInstanceInSecond < currentTimeStamp {
					delete(instance.ScannedTxID, txID)
				}
			}
		}

		for idx, instance := range newlyTrackingInstance {
			_, exists := btcAddressIndexMapping[instance.BTCAddress]
			if exists {
				continue
			}

			address, err := btcutil.DecodeAddress(instance.BTCAddress, b.chainCfg)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not decode address %v - with err: %v", instance.BTCAddress, err))
				continue
			}

			shieldingMonitoringList = append(shieldingMonitoringList, instance)
			trackingBTCAddress = append(trackingBTCAddress, address)
			btcAddressIndexMapping[instance.BTCAddress] = idx
		}

		listUnspentResults, err := b.btcClient.ListUnspentMinMaxAddresses(BTCConfirmationThreshold, 99999999, trackingBTCAddress)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not scan list of unspent outcoins - with err: %v", err))
			continue
		}
		for _, unspentCoins := range listUnspentResults {
			btcAddress := unspentCoins.Address
			idx := btcAddressIndexMapping[btcAddress]
			incAddress := shieldingMonitoringList[idx].IncAddress

			txHash := unspentCoins.TxID
			_, exists := shieldingMonitoringList[idx].ScannedTxID[txHash]
			if exists {
				continue
			}
			shieldingMonitoringList[idx].ScannedTxID[txHash] = currentTimeStamp

			txID, _ := chainhash.NewHashFromStr(txHash)
			tx, _ := b.btcClient.GetRawTransactionVerbose(txID)
			blkHash, _ := chainhash.NewHashFromStr(tx.BlockHash)
			blk, err := b.btcClient.GetBlockHeaderVerbose(blkHash)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not get block height of block hash: %v - with err: %v", blkHash, err))
				continue
			}

			// generate proof
			proof, err := utils.BuildProof(b.btcClient, txHash, uint64(blk.Height))
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not build proof for tx: %v - with err: %v", txID, err))
				continue
			}

			fmt.Printf("Found shielding request for address %v, with BTC tx %v\n", incAddress, txID)
			waitingShieldingList[txHash] = &ShieldingRequestInfo{
				IncAddress:     incAddress,
				Proof:          proof,
				BTCBlockHeight: uint64(blk.Height),
			}
		}

		// send shielding request RPC to Incognito chain
		relayingBTCHeight, err := b.getLatestBTCBlockHashFromIncog()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}
		fmt.Printf("Number of waiting shielding requests: %v\n", len(waitingShieldingList))
		fmt.Printf("Number of shielding monitoring instances: %v\n", len(shieldingMonitoringList))

		sentShieldingRequest := make(chan string, len(waitingShieldingList))
		var wg sync.WaitGroup
		for txHash, value := range waitingShieldingList {
			if value.BTCBlockHeight+BTCConfirmationThreshold <= relayingBTCHeight {
				// send RPC
				txID, err := b.submitShieldingRequest(value.IncAddress, value.Proof)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not send shielding request from BTC tx %v proof with err: %v", txHash, err))
					continue
				}
				fmt.Printf("Shielding txID: %v\n", txID)
				wg.Add(1)
				curTxHash := txHash
				go func() {
					defer wg.Done()
					status, err := b.getRequestShieldingStatus(txID)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not get request shielding status from BTC tx %v - with err: %v", curTxHash, err))
					} else {
						if status == 0 { // rejected
							b.ExportErrorLog(fmt.Sprintf("Request shielding failed BTC tx %v, shielding txID %v", curTxHash, txID))
						} else {
							b.ExportInfoLog(fmt.Sprintf("Request shielding succeed BTC tx %v, shielding txID %v", curTxHash, txID))
						}
						sentShieldingRequest <- curTxHash
					}
				}()
			}
		}
		wg.Wait()

		close(sentShieldingRequest)
		for txHash := range sentShieldingRequest {
			delete(waitingShieldingList, txHash)
		}

		shieldingTxArrayObjectBytes, _ := json.Marshal(&ShieldingTxArrayObject{
			ShieldingMonitoringList: shieldingMonitoringList,
			WaitingShieldingList:    waitingShieldingList,
			LastTimeStampUpdated:    lastTimeUpdated,
		})
		err = b.db.Put([]byte("BTCMonitor-LastUpdate"), shieldingTxArrayObjectBytes, nil)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}
		time.Sleep(15 * time.Second)
	}
}
