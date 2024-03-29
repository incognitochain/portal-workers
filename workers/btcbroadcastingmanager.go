package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	MaxUnshieldFee                  = 500000
	InitIncBlockBatchSize           = 100
	FirstBroadcastTxBlockHeight     = 1427305
	TimeoutBTCFeeReplacement        = 80
	TimeIntervalBTCFeeReplacement   = 10
	BroadcastingManagerDBFileDir    = "db/broadcastingmanager"
	BroadcastingManagerDBObjectName = "BTCBroadcast-LastUpdate"
)

type BTCBroadcastingManager struct {
	WorkerAbs
	btcClient  *rpcclient.Client
	bitcoinFee uint
	db         *leveldb.DB
}

type BroadcastTx struct {
	TxContent     string // only has value when be broadcasted
	TxHash        string // only has value when be broadcasted
	VSize         int
	RBFReqTxID    string
	FeePerRequest uint
	NumOfRequests uint
	IsBroadcasted bool
	BlkHeight     uint64 // height of the current Incog chain height when broadcasting tx
}

type BroadcastTxArrayObject struct {
	TxArray       map[string]map[string]*BroadcastTx // key: batchID | RBFRexTxID
	NextBlkHeight uint64                             // height of the next block need to scan in Inc chain
}

func (b *BTCBroadcastingManager) Init(
	id int, name string, freq int, network string, utxoManager *utxomanager.UTXOManager,
) error {
	b.WorkerAbs.Init(id, name, freq, network, utxoManager)

	var err error

	// init bitcoin rpcclient
	b.btcClient, err = utils.BuildBTCClient()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not initialize Bitcoin RPCClient - with err: %v", err))
		return err
	}

	return nil
}

func (b *BTCBroadcastingManager) ExportErrorLog(msg string) {
	b.WorkerAbs.ExportErrorLog(msg)
}

func (b *BTCBroadcastingManager) ExportInfoLog(msg string) {
	b.WorkerAbs.ExportInfoLog(msg)
}

// This function will execute a worker that has 3 main tasks:
// - Broadcast a unshielding transaction to Bitcoin network
// - Check for a Bitcoin transaction is stuck or not and request RBF transaction
// - Check a broadcasted Bitcoin transaction confirmation and notify the Incognito chain
func (b *BTCBroadcastingManager) Execute() {
	b.ExportErrorLog("BTCBroadcastingManager worker is executing...")
	// init leveldb instance
	var err error
	b.db, err = leveldb.OpenFile(BroadcastingManagerDBFileDir, nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return
	}
	defer b.db.Close()

	nextBlkHeight := uint64(FirstBroadcastTxBlockHeight)
	broadcastTxArray := map[string]map[string]*BroadcastTx{}

	// restore from db
	lastUpdateBytes, err := b.db.Get([]byte(BroadcastingManagerDBObjectName), nil)
	if err == nil {
		var broadcastTxsDBObject *BroadcastTxArrayObject
		json.Unmarshal(lastUpdateBytes, &broadcastTxsDBObject)
		nextBlkHeight = broadcastTxsDBObject.NextBlkHeight
		broadcastTxArray = broadcastTxsDBObject.TxArray
	} else {
		b.ExportInfoLog("Restart without DB")
	}

	shardID, _ := strconv.Atoi(os.Getenv("SHARD_ID"))

	for {
		isBTCNodeAlive := getBTCFullnodeStatus(b.btcClient)
		if !isBTCNodeAlive {
			b.ExportErrorLog("Could not connect to BTC full node")
			return
		}

		feePerVByte, err := getBitcoinFee()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get bitcoin fee, error: %v", err))
			return
		}
		b.bitcoinFee = uint(feePerVByte)

		fmt.Printf("Current relaying fee: %v\n", b.bitcoinFee)

		// wait until next blocks available
		var curIncBlkHeight uint64
		for {
			curIncBlkHeight, err = getFinalizedBlockHeightByShardID(b.UTXOManager.IncClient, b.Logger, -1)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not get latest beacon height - with err: %v", err))
				return
			}
			if nextBlkHeight <= curIncBlkHeight {
				break
			}
			time.Sleep(40 * time.Second)
		}

		// var IncBlockBatchSize uint64
		// if nextBlkHeight+InitIncBlockBatchSize-1 <= curIncBlkHeight {
		// 	IncBlockBatchSize = InitIncBlockBatchSize
		// } else {
		// 	IncBlockBatchSize = curIncBlkHeight - nextBlkHeight + 1
		// }

		// fmt.Printf("Next Scan Block Height: %v, Batch Size: %v, Current Finalized Block Height: %v\n", nextBlkHeight, IncBlockBatchSize, curIncBlkHeight)

		// Only need to get the batchID from the lastest beacon height
		batchIDs, err := getBatchIDsFromBeaconHeight(curIncBlkHeight, b.RPCClient, b.Logger, uint64(FirstBroadcastTxBlockHeight))
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve batches from beacon block %v - with err: %v", curIncBlkHeight, err))
			return
		}
		newBroadcastTxArray, err := b.getBroadcastTx(broadcastTxArray, batchIDs, curIncBlkHeight)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve broadcast txs - with err: %v", err))
			return
		}

		for batchID, batchInfo := range newBroadcastTxArray {
			for _, tx := range batchInfo {
				if tx.IsBroadcasted {
					fmt.Printf("Broadcast tx for batch %v, content %v \n", batchID, tx.TxContent)
					err := b.broadcastTx(tx.TxContent)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not broadcast tx %v - with err: %v", tx.TxHash, err))
						continue
					}
				} else {
					fmt.Printf("Does not broadcast tx for batch %v has fee %v is not enough\n", batchID, tx.FeePerRequest)
				}
			}
		}

		broadcastTxArray = joinTxArray(broadcastTxArray, newBroadcastTxArray)

		// check confirmed -> send rpc to notify the Inc chain
		relayingBTCHeight, err := getLatestBTCHeightFromIncog(b.RPCBTCRelayingReaders)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}

		var wg sync.WaitGroup

		// submit confirmation requests by checking BTC tx
		for batchID, txArray := range broadcastTxArray {
			for _, tx := range txArray {
				curBatchID := batchID
				curTx := tx

				isConfirmed, btcBlockHeight := b.isConfirmedBTCTx(curTx.TxHash)

				if isConfirmed && btcBlockHeight+BTCConfirmationThreshold-1 <= relayingBTCHeight {
					fmt.Printf("BTC Tx %v is confirmed\n", curTx.TxHash)

					// submit confirmed tx
					wg.Add(1)
					go func() {
						defer wg.Done()

						// generate BTC proof
						btcProof, err := utils.BuildProof(b.btcClient, curTx.TxHash, btcBlockHeight)
						if err != nil {
							b.ExportErrorLog(fmt.Sprintf("Could not generate BTC proof for batch %v - with err: %v", curBatchID, err))
							return
						}
						txID, err := b.submitConfirmedTx(btcProof, curBatchID)
						if err != nil {
							b.ExportErrorLog(fmt.Sprintf("Could not submit confirmed tx for batch %v - with err: %v", curBatchID, err))
							return
						}

						status, err := b.getSubmitConfirmedTxStatus(txID)
						if err != nil {
							b.ExportErrorLog(fmt.Sprintf("Could not get submit confirmed tx status for batch %v, txID %v - with err: %v", curBatchID, txID, err))
						} else {
							ok := isFinalizedTx(b.UTXOManager.IncClient, b.Logger, shardID, txID)
							if !ok {
								return
							}
							if status == 0 { // rejected
								b.ExportErrorLog(fmt.Sprintf("Send confirmation failed for batch %v, txID %v", curBatchID, txID))
							} else { // succeed
								b.ExportInfoLog(fmt.Sprintf("Send confirmation succeed for batch %v, txID %v", curBatchID, txID))
							}
						}
					}()
				}
			}
		}
		wg.Wait()

		confirmedBatchIDChan := make(chan string, len(broadcastTxArray))
		// check whether unshielding batches are completed by batch ID
		for batchID := range broadcastTxArray {
			curBatchID := batchID
			wg.Add(1)
			go func() {
				defer wg.Done()
				status, err := b.getUnshieldingBatchStatus(curBatchID)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not get batch %v status - with err: %v", curBatchID, err))
				} else if status.Status == 1 { // completed
					b.ExportInfoLog(fmt.Sprintf("Batch %v is completed", curBatchID))
					confirmedBatchIDChan <- curBatchID
				}
			}()
		}
		wg.Wait()

		close(confirmedBatchIDChan)
		for batchID := range confirmedBatchIDChan {
			delete(broadcastTxArray, batchID)
		}

		// check if waiting too long -> send rpc to notify the Inc chain for fee replacement
		for batchID, txArray := range broadcastTxArray {
			tx := getLastestBroadcastTx(txArray)
			curBatchID := batchID
			curTx := tx

			if b.isTimeoutBTCTx(curTx, curIncBlkHeight) { // waiting too long
				wg.Add(1)
				go func() {
					defer wg.Done()
					newFee := utils.GetNewFee(curTx.VSize, curTx.FeePerRequest, curTx.NumOfRequests, b.bitcoinFee)
					if newFee > MaxUnshieldFee {
						if curTx.FeePerRequest < MaxUnshieldFee {
							newFee = MaxUnshieldFee
						} else {
							// // re-broadcast tx
							// err := b.broadcastTx(curTx.TxContent)
							// if err != nil {
							// 	b.ExportErrorLog(fmt.Sprintf("Could not broadcast tx %v - with err: %v", tx.TxHash, err))
							// }
							return
						}
					}
					fmt.Printf("Old fee %v, request new fee %v for batchID %v\n", curTx.FeePerRequest, newFee, curBatchID)
					// notify the Inc chain for fee replacement
					txID, err := b.requestFeeReplacement(curBatchID, newFee)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not request RBF for batch %v - with err: %v", curBatchID, err))
						return
					}

					status, err := b.getRequestFeeReplacementTxStatus(txID)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not request RBF tx status for batch %v, txID %v - with err: %v", curBatchID, txID, err))
					} else {
						ok := isFinalizedTx(b.UTXOManager.IncClient, b.Logger, shardID, txID)
						if !ok {
							return
						}
						if status == 0 { // rejected
							b.ExportErrorLog(fmt.Sprintf("Send RBF request failed for batch %v, txID %v", curBatchID, txID))
						} else {
							b.ExportInfoLog(fmt.Sprintf("Send RBF request succeed for batch %v, txID %v", curBatchID, txID))
						}
					}
				}()
			}
		}
		wg.Wait()

		nextBlkHeight = curIncBlkHeight + 1

		// update to db
		BroadcastTxArrayObjectBytes, _ := json.Marshal(&BroadcastTxArrayObject{
			TxArray:       broadcastTxArray,
			NextBlkHeight: nextBlkHeight,
		})
		err = b.db.Put([]byte(BroadcastingManagerDBObjectName), BroadcastTxArrayObjectBytes, nil)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}

		sleepingTime := 100
		fmt.Printf("Sleeping: %v seconds\n", sleepingTime)
		time.Sleep(time.Duration(sleepingTime) * time.Second)
	}
}
