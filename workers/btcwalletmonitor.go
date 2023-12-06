package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/0xkraken/btcd/chaincfg"
	"github.com/0xkraken/btcd/chaincfg/chainhash"
	"github.com/0xkraken/btcd/rpcclient"
	"github.com/0xkraken/btcutil"
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

type ShieldingMonitoringInfo struct {
	IncAddress  string
	BTCAddress  string
	TimeStamp   int64
	ScannedTxID map[string]int64
}

type ShieldingRequestInfo struct {
	TxHash         string
	IncAddress     string
	Proof          string
	BTCBlockHeight uint64
}

type ShieldingTxArrayObject struct {
	ShieldingMonitoringList   []*ShieldingMonitoringInfo
	WaitingShieldingList      map[string]*ShieldingRequestInfo // key: proofHash
	LastTimeStampUpdated      int64
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

// This function will execute a worker that has 2 main tasks:
// - Monitor Bitcoin multisig wallets that corresponding with Incognito Wallet App users
// - Send shielding request on behalf of users to Incognito chain
func (b *BTCWalletMonitor) Execute() {
	b.ExportErrorLog("BTCWalletMonitor worker is executing...")
	// init leveldb instance
	var err error
	b.db, err = leveldb.OpenFile(WalletMonitorDBFileDir, nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return
	}
	defer b.db.Close()

	shieldingMonitoringList := []*ShieldingMonitoringInfo{}
	waitingShieldingList := map[string]*ShieldingRequestInfo{}
	lastTimeUpdated := int64(0)
	lastScannedBTCBlockHeight := int64(FirstScannedBTCBlkHeight - 1)

	// restore data from db
	lastUpdateBytes, err := b.db.Get([]byte(WalletMonitorDBObjectName), nil)
	if err == nil {
		var shieldingTxArrayObject *ShieldingTxArrayObject
		json.Unmarshal(lastUpdateBytes, &shieldingTxArrayObject)
		shieldingMonitoringList = shieldingTxArrayObject.ShieldingMonitoringList
		waitingShieldingList = shieldingTxArrayObject.WaitingShieldingList
		lastTimeUpdated = shieldingTxArrayObject.LastTimeStampUpdated
		lastScannedBTCBlockHeight = shieldingTxArrayObject.LastScannedBTCBlockHeight
	}

	trackingBTCAddresses := []btcutil.Address{}
	btcAddressIndexMapping := map[string]int{}
	for idx, instance := range shieldingMonitoringList {
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
		isBTCNodeAlive := getBTCFullnodeStatus(b.btcClient)
		if !isBTCNodeAlive {
			b.ExportErrorLog("Could not connect to BTC full node")
			return
		}

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

			shieldingMonitoringList = append(shieldingMonitoringList, instance)
			trackingBTCAddresses = append(trackingBTCAddresses, address)
			btcAddressIndexMapping[instance.BTCAddress] = len(shieldingMonitoringList) - 1
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
		// Over confirmation just for sure
		maxConfirmation := int(btcBestBlockHeight - lastScannedBTCBlockHeight + 144)
		if maxConfirmation < minConfirmation {
			maxConfirmation = minConfirmation
		}
		listUnspentResults, err := b.btcClient.ListUnspentMinMaxAddresses(minConfirmation, maxConfirmation, trackingBTCAddresses)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not scan list of unspent outcoins - with err: %v", err))
			return
		}
		lastScannedBTCBlockHeight = btcBestBlockHeight - int64(minConfirmation) + 1

		var wg sync.WaitGroup
		shieldingRequestChan := make(chan map[string]*ShieldingRequestInfo, len(listUnspentResults))
		for _, unspentCoins := range listUnspentResults {
			btcAddress := unspentCoins.Address
			idx := btcAddressIndexMapping[btcAddress]
			incAddress := shieldingMonitoringList[idx].IncAddress

			txHash := unspentCoins.TxID
			_, exists := shieldingMonitoringList[idx].ScannedTxID[txHash]
			if exists || convertBTCtoNanopBTC(unspentCoins.Amount) < MinShieldAmount {
				continue
			}
			shieldingMonitoringList[idx].ScannedTxID[txHash] = currentTimeStamp

			wg.Add(1)
			go func() {
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

				fmt.Printf("Found shielding request for address %v, with BTC tx %v\n", incAddress, txHash)
				proofHash := hashProof(proof, incAddress)
				shieldingRequestChan <- map[string]*ShieldingRequestInfo{
					proofHash: {
						TxHash:         txHash,
						IncAddress:     incAddress,
						Proof:          proof,
						BTCBlockHeight: uint64(blk.Height),
					},
				}
			}()
		}
		wg.Wait()

		close(shieldingRequestChan)
		for shieldRequest := range shieldingRequestChan {
			for key, value := range shieldRequest {
				waitingShieldingList[key] = value
			}
		}

		// send shielding request RPC to Incognito chain
		relayingBTCHeight, err := getLatestBTCHeightFromIncog(b.RPCBTCRelayingReaders)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}
		fmt.Printf("Number of waiting shielding requests: %v\n", len(waitingShieldingList))
		fmt.Printf("Number of shielding monitoring instances: %v\n", len(shieldingMonitoringList))

		sentShieldingRequest := make(chan string, len(waitingShieldingList))
		for proofHash, value := range waitingShieldingList {
			if value.BTCBlockHeight+BTCConfirmationThreshold-1 <= relayingBTCHeight {
				wg.Add(1)
				curProofHash := proofHash
				curTxHash := value.TxHash
				curValue := value
				go func() {
					defer wg.Done()
					// send RPC
					if curTxHash == "5f07c905a3d449e8cb50976a5968571df99be16711e68e69ad056d4f3800057d" ||
						curTxHash == "c7ad72656b636e1c6ae00f1855724cf3d0a14b01a209a6e7a5843a7869816b03" ||
						curTxHash == "a936943e6fccf2843eec4e4421d4549f64a323f68dcfa1df56f75985b54588fb" ||
						curTxHash == "c98620b0583877173de3abc9123c537014d89fd822fa59ec9b45b1d0acb95d33" ||
						curTxHash == "d9babf6154deb0aff98dd766582bb8ab423681c4c681649c65a5cdc303621f29" ||
						curTxHash == "9f0ae7a7a9359328914993ac393caa58e93fb4c4eb2ba96ac0dcdc095880dbba" ||
						curTxHash == "ed73f37116d2f0e5abe7c8e67c18570aae7c73bf04a374a7ca684b306b9ec29a" ||
						curTxHash != "be7107cbae2b0b8379dce0c7aec189ad728125384a0701b017e2e845701ff1f3" {
						b.ExportErrorLog(fmt.Sprintf("Ignore shielding BTC TxID: %v", curTxHash))
						sentShieldingRequest <- curProofHash
						return
					}
					txID, err := b.submitShieldingRequest(curValue.IncAddress, curValue.Proof)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not send shielding request from BTC tx %v proof for incAddress %v with err: %v", curTxHash, curValue.IncAddress, err))
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
								sentShieldingRequest <- curProofHash
							} else {
								b.ExportErrorLog(fmt.Sprintf("Request shielding failed BTC tx %v, shielding txID %v - invalid proof", curTxHash, txID))
							}
						} else {
							res, err := sendAirdropRequest(curValue.IncAddress)
							if err == nil {
								b.ExportInfoLog(fmt.Sprintf("Request shielding succeed BTC tx %v, shielding txID %v, airdrop status: %v", curTxHash, txID, res))
							} else {
								b.ExportInfoLog(fmt.Sprintf("Request shielding succeed BTC tx %v, shielding txID %v, airdrop failed - %v", curTxHash, txID, err))
							}

							sentShieldingRequest <- curProofHash
						}

					}
				}()
			}
		}
		wg.Wait()

		close(sentShieldingRequest)
		for proofHash := range sentShieldingRequest {
			delete(waitingShieldingList, proofHash)
		}

		shieldingTxArrayObjectBytes, _ := json.Marshal(&ShieldingTxArrayObject{
			ShieldingMonitoringList:   shieldingMonitoringList,
			WaitingShieldingList:      waitingShieldingList,
			LastTimeStampUpdated:      lastTimeUpdated,
			LastScannedBTCBlockHeight: lastScannedBTCBlockHeight,
		})
		err = b.db.Put([]byte(WalletMonitorDBObjectName), shieldingTxArrayObjectBytes, nil)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}
		time.Sleep(100 * time.Second)
	}
}
