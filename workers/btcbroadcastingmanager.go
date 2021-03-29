package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/blockcypher/gobcy"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/syndtr/goleveldb/leveldb"
)

const InitIncBlockBatchSize = 1000
const FirstBroadcastTxBlockHeight = 1
const TimeoutBTCFeeReplacement = 50
const ProcessedBlkCacheDepth = 10000

type BTCBroadcastingManager struct {
	WorkerAbs
	bcy      gobcy.API
	bcyChain gobcy.Blockchain
	db       *leveldb.DB
}

type BroadcastTx struct {
	TxContent     string
	TxHash        string
	FeePerRequest uint
	NumOfRequests uint
	BlkHeight     uint64 // height of the current Incog chain height when broadcasting tx
}

type FeeReplacementTx struct {
	ReqTxID       string
	FeePerRequest uint
	NumOfRequests uint
	BlkHeight     uint64
}

type ConfirmedTx struct {
	BlkHeight uint64
}

type BroadcastTxArrayObject struct {
	TxArray               map[string]*BroadcastTx
	FeeReplacementTxArray map[string]*FeeReplacementTx
	ConfirmedTxArray      map[string]*ConfirmedTx
	NextBlkHeight         uint64 // height of the next block need to scan in Inc chain
}

func (b *BTCBroadcastingManager) Init(id int, name string, freq int, network string) error {
	err := b.WorkerAbs.Init(id, name, freq, network)
	// init blockcypher instance
	b.bcy = gobcy.API{Token: os.Getenv("BLOCKCYPHER_TOKEN"), Coin: "btc", Chain: b.GetNetwork()}
	b.bcyChain, err = b.bcy.GetChain()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get btc chain info from cypher api - with err: %v", err))
		return err
	}

	// init leveldb instance
	b.db, err = leveldb.OpenFile("db/broadcastingmanager", nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return err
	}

	return nil
}

func (b *BTCBroadcastingManager) ExportErrorLog(msg string) {
	b.WorkerAbs.ExportErrorLog(msg)
}

func (b *BTCBroadcastingManager) Execute() {
	b.Logger.Info("BTCBroadcastingManager worker is executing...")
	defer b.db.Close()

	nextBlkHeight := uint64(FirstBroadcastTxBlockHeight)
	broadcastTxArray := map[string]*BroadcastTx{}           // key: batchID
	feeReplacementTxArray := map[string]*FeeReplacementTx{} // key: batchID
	confirmedTxArray := map[string]*ConfirmedTx{}           // key: batchID

	// restore from db
	lastUpdateBytes, err := b.db.Get([]byte("BTCBroadcast-LastUpdate"), nil)
	if err == nil {
		var broadcastTxsDBObject *BroadcastTxArrayObject
		json.Unmarshal(lastUpdateBytes, &broadcastTxsDBObject)
		nextBlkHeight = broadcastTxsDBObject.NextBlkHeight
		broadcastTxArray = broadcastTxsDBObject.TxArray
		feeReplacementTxArray = broadcastTxsDBObject.FeeReplacementTxArray
		confirmedTxArray = broadcastTxsDBObject.ConfirmedTxArray
	}

	for {
		curIncBlkHeight, err := b.getLatestBeaconHeight()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get latest beacon height - with err: %v", err))
			return
		}

		// wait until next block available
		for nextBlkHeight >= curIncBlkHeight {
			time.Sleep(10 * time.Second)
			curIncBlkHeight, err = b.getLatestBeaconHeight()
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not get latest beacon height - with err: %v", err))
				return
			}
		}

		var IncBlockBatchSize uint64
		if nextBlkHeight+InitIncBlockBatchSize <= curIncBlkHeight { // load until the final view
			IncBlockBatchSize = InitIncBlockBatchSize
		} else {
			IncBlockBatchSize = curIncBlkHeight - nextBlkHeight
		}

		fmt.Printf("Next Scan Block Height: %v, Batch Size: %v, Current Block Height: %v\n", nextBlkHeight, IncBlockBatchSize, curIncBlkHeight)

		// remove too old processed transactions
		for batchID, value := range confirmedTxArray {
			if value.BlkHeight+ProcessedBlkCacheDepth < curIncBlkHeight {
				delete(confirmedTxArray, batchID)
			}
		}

		for batchID, value := range feeReplacementTxArray {
			if value.BlkHeight+ProcessedBlkCacheDepth < curIncBlkHeight {
				delete(confirmedTxArray, batchID)
			}
		}

		// get list of processed batch IDs
		processedBatchIDs := map[string]bool{}
		for batchID := range broadcastTxArray {
			processedBatchIDs[batchID] = true
		}
		for batchID := range feeReplacementTxArray {
			processedBatchIDs[batchID] = true
		}
		for batchID := range confirmedTxArray {
			processedBatchIDs[batchID] = true
		}

		var tempBroadcastTxArray1 map[string]*BroadcastTx
		var tempBroadcastTxArray2 map[string]*BroadcastTx
		tempBroadcastTxArray1, err = b.getBroadcastTxsFromBeaconHeight(processedBatchIDs, nextBlkHeight+IncBlockBatchSize-1, curIncBlkHeight)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Incognito block - with err: %v", err))
			return
		}
		feeReplacementTxArray, tempBroadcastTxArray2, err = b.getBroadcastReplacementTx(feeReplacementTxArray, curIncBlkHeight)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve get broadcast txs - with err: %v", err))
			return
		}

		tempBroadcastTxArray := joinTxArray(tempBroadcastTxArray1, tempBroadcastTxArray2)

		for batchID, tx := range tempBroadcastTxArray {
			fmt.Printf("Broadcast tx for batch %v, content %v \n", batchID, tx.TxContent)
			err := b.broadcastTx(tx.TxContent)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not broadcast tx %v - with err: %v", tx.TxHash, err))
				continue
			}
		}
		broadcastTxArray = joinTxArray(broadcastTxArray, tempBroadcastTxArray)

		// check confirmed -> send rpc to notify the Inc chain
		relayingBTCHeight, err := b.getLatestBTCBlockHashFromIncog()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}

		var wg sync.WaitGroup

		confirmedBatchIDChan := make(chan map[string]*ConfirmedTx, len(broadcastTxArray))
		for batchID, tx := range broadcastTxArray {
			curBatchID := batchID
			curTx := tx

			isConfirmed, btcBlockHeight := b.isConfirmedBTCTx(curTx.TxHash)

			if isConfirmed && btcBlockHeight+BTCConfirmationThreshold <= relayingBTCHeight {
				// generate BTC proof
				btcProof, err := b.buildProof(curTx.TxHash, btcBlockHeight)
				fmt.Printf("%+v\n", btcProof)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not generate BTC proof for batch %v - with err: %v", curBatchID, err))
					continue
				}

				// submit confirmed tx
				wg.Add(1)
				go func() {
					defer wg.Done()
					txID, err := b.submitConfirmedTx(btcProof, curBatchID)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not submit confirmed tx for batch %v - with err: %v", curBatchID, err))
						return
					}
					err = b.getSubmitConfirmedTxStatus(txID)
					if err == nil {
						confirmedBatchIDChan <- map[string]*ConfirmedTx{
							curBatchID: {
								BlkHeight: curIncBlkHeight,
							},
						}
					} else {
						b.ExportErrorLog(fmt.Sprintf("Could not get submit confirmed tx status for batch %v - with err: %v", curBatchID, err))
						return
					}
				}()
			}
		}
		wg.Wait()

		close(confirmedBatchIDChan)
		for batch := range confirmedBatchIDChan {
			for batchID, tx := range batch {
				confirmedTxArray[batchID] = tx
				delete(broadcastTxArray, batchID)
			}
		}

		// check if waiting too long -> send rpc to notify the Inc chain for fee replacement
		replacedBatchIDChan := make(chan map[string]*FeeReplacementTx, len(broadcastTxArray))
		for batchID, tx := range broadcastTxArray {
			curBatchID := batchID
			curTx := tx

			if b.isTimeoutBTCTx(curTx, curIncBlkHeight) { // waiting too long
				newFee, err := utils.GetNewFee(len(curTx.TxContent), curTx.FeePerRequest, curTx.NumOfRequests)
				fmt.Printf("Old fee %v, request new fee %v for batchID %v\n", curTx.FeePerRequest, newFee, curBatchID)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not get new fee for batch %v - with err: %v", curBatchID, err))
					continue
				}
				// notify the Inc chain for fee replacement
				wg.Add(1)
				go func() {
					defer wg.Done()
					txID, err := b.requestFeeReplacement(curBatchID, newFee)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not request fee replacement for batch %v - with err: %v", curBatchID, err))
						return
					}
					err = b.getRequestFeeReplacementTxStatus(txID)
					if err == nil {
						replacedBatchIDChan <- map[string]*FeeReplacementTx{
							batchID: {
								ReqTxID:       txID,
								FeePerRequest: newFee,
								NumOfRequests: curTx.NumOfRequests,
								BlkHeight:     curIncBlkHeight,
							},
						}
					} else {
						b.ExportErrorLog(fmt.Sprintf("Could not get request fee replacement tx status for batch %v, txID %v - with err: %v", curBatchID, txID, err))
						return
					}
				}()
			}
		}
		wg.Wait()

		close(replacedBatchIDChan)
		for batch := range replacedBatchIDChan {
			for batchID, tx := range batch {
				feeReplacementTxArray[batchID] = tx
				delete(broadcastTxArray, batchID)
			}
		}

		nextBlkHeight += IncBlockBatchSize

		// update to db
		BroadcastTxArrayObjectBytes, _ := json.Marshal(&BroadcastTxArrayObject{
			TxArray:               broadcastTxArray,
			ConfirmedTxArray:      confirmedTxArray,
			FeeReplacementTxArray: feeReplacementTxArray,
			NextBlkHeight:         nextBlkHeight,
		})
		err = b.db.Put([]byte("BTCBroadcast-LastUpdate"), BroadcastTxArrayObjectBytes, nil)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}

		time.Sleep(2 * time.Second)
	}
}
