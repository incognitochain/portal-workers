package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/blockcypher/gobcy"
	"github.com/syndtr/goleveldb/leveldb"
)

const TimeoutTrackingInstanceInSecond = int64(2 * 60 * 60)

type BTCWalletMonitor struct {
	WorkerAbs
	bcy      gobcy.API
	bcyChain gobcy.Blockchain
	db       *leveldb.DB
}

type ShieldingMonitoringInfo struct {
	IncAddress         string
	BTCAddress         string
	TimeStamp          int64
	LastBTCHeightTrack uint64
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
	err := b.WorkerAbs.Init(id, name, freq, network)
	// init blockcypher instance
	b.bcy = gobcy.API{Token: os.Getenv("BLOCKCYPHER_TOKEN"), Coin: "btc", Chain: b.GetNetwork()}
	b.bcyChain, err = b.bcy.GetChain()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get btc chain info from cypher api - with err: %v", err))
		return err
	}

	// init leveldb instance
	b.db, err = leveldb.OpenFile("db/walletmonitor", nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return err
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

	// shieldingMonitoringList = append(shieldingMonitoringList, &ShieldingMonitoringInfo{
	// 	IncAddress:         "12S5Lrs1XeQLbqN4ySyKtjAjd2d7sBP2tjFijzmp6avrrkQCNFMpkXm3FPzj2Wcu2ZNqJEmh9JriVuRErVwhuQnLmWSaggobEWsBEci",
	// 	BTCAddress:         "tb1qxds4waaq7zll6w699sfarajvm3c5m98qpttkcvcgesxwk0989pks42hktr",
	// 	TimeStamp:          time.Now().Unix(),
	// 	LastBTCHeightTrack: 1970196,
	// })

	for {
		// get new rescanning instance from API
		currentTimeStamp := time.Now().Unix()
		newlyTrackingInstance, err := b.getTrackingInstance(lastTimeUpdated, currentTimeStamp)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get tracking instance from API - with err: %v", err))
			return
		}
		lastTimeUpdated = currentTimeStamp
		shieldingMonitoringList = append(shieldingMonitoringList, newlyTrackingInstance...)

		// delete timeout tracking instance
		idx := 0
		lenArr := len(shieldingMonitoringList)
		for idx < lenArr {
			if shieldingMonitoringList[idx].TimeStamp+TimeoutTrackingInstanceInSecond < currentTimeStamp {
				// delete tracking instance
				shieldingMonitoringList[idx], shieldingMonitoringList[lenArr-1] = shieldingMonitoringList[lenArr-1], shieldingMonitoringList[idx]
				lenArr--
			} else {
				idx++
			}
		}
		shieldingMonitoringList = shieldingMonitoringList[:lenArr]

		// track transactions send to bitcoin addresses
		for _, trackingInstance := range shieldingMonitoringList {
			btcAddress := trackingInstance.BTCAddress
			incAddress := trackingInstance.IncAddress
			lastBTCHeightTracked := trackingInstance.LastBTCHeightTrack

			addrInfo, err := b.bcy.GetAddr(btcAddress, map[string]string{
				"after":         strconv.FormatUint(lastBTCHeightTracked+1, 10),
				"confirmations": strconv.FormatInt(int64(BTCConfirmationThreshold), 10),
				"unspentOnly":   "true",
			})
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not retrieve tx to address %v - with err: %v", btcAddress, err))
				continue
			}

			for _, tx := range addrInfo.TXRefs {
				if tx.BlockHeight <= 0 {
					continue
				}

				time.Sleep(1 * time.Second)
				// update last btc block height tracked
				if uint64(tx.BlockHeight) > lastBTCHeightTracked {
					lastBTCHeightTracked = uint64(tx.BlockHeight)
				}

				if tx.TXOutputN != -1 {
					// generate proof
					proof, err := b.buildProof(tx.TXHash, uint64(tx.BlockHeight))
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not build proof for tx: %v - with err: %v", tx.TXHash, err))
						continue
					}

					fmt.Printf("Found shielding request for address %v, with BTC tx %v\n", incAddress, tx.TXHash)
					waitingShieldingList[tx.TXHash] = &ShieldingRequestInfo{
						IncAddress:     incAddress,
						Proof:          proof,
						BTCBlockHeight: uint64(tx.BlockHeight),
					}
				}
			}

			trackingInstance.LastBTCHeightTrack = lastBTCHeightTracked
			time.Sleep(1 * time.Second)
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
