package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/blockcypher/gobcy"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/syndtr/goleveldb/leveldb"
)

const InitIncBlockBatchSize = 1000
const FirstBroadcastTxBlockHeight = 1
const TimeoutBTCFeeReplacement = 100
const ConfirmationThreshold = 6
const ProcessedBlkCacheDepth = 2000

type BTCBroadcastingManager struct {
	WorkerAbs
	bcy      gobcy.API
	bcyChain gobcy.Blockchain
	db       *leveldb.DB
}

type BroadcastTx struct {
	TxContent     string
	TxHash        string
	BatchID       string
	FeePerRequest uint
	NumOfRequests uint
	BlkHeight     uint64 // height of the broadcast tx
}

type FeeReplacementTx struct {
	ReqTxID   string
	BatchID   string
	BlkHeight uint64
}

type ConfirmedTx struct {
	BatchID   string
	BlkHeight uint64
}

type BroadcastTxsBlock struct {
	TxArray   []*BroadcastTx
	BlkHeight uint64
	Err       error
}

type BroadcastTxArrayObject struct {
	TxArray               []*BroadcastTx
	FeeReplacementTxArray []*FeeReplacementTx
	ConfirmedTxArray      []*ConfirmedTx
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
	b.db, err = leveldb.OpenFile("db", nil)
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
	b.Logger.Info("BTCBroadcastingManager agent is executing...")
	defer b.db.Close()

	nextBlkHeight := uint64(FirstBroadcastTxBlockHeight)
	broadcastTxArray := []*BroadcastTx{}
	feeReplacementTxArray := []*FeeReplacementTx{}
	confirmedTxArray := []*ConfirmedTx{}

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
			IncBlockBatchSize = 1
		}

		fmt.Printf("Next Scan Block Height: %v, Batch Size: %v, Current Block Height: %v\n", nextBlkHeight, IncBlockBatchSize, curIncBlkHeight)

		var idx int
		var lenArray int

		// remove too old processed transactions
		idx = 0
		lenArray = len(confirmedTxArray)
		for idx < lenArray {
			if confirmedTxArray[idx].BlkHeight+ProcessedBlkCacheDepth < curIncBlkHeight {
				confirmedTxArray[lenArray-1], confirmedTxArray[idx] = confirmedTxArray[idx], confirmedTxArray[lenArray-1]
				lenArray--
			} else {
				idx++
			}
		}
		confirmedTxArray = confirmedTxArray[:lenArray]

		idx = 0
		lenArray = len(feeReplacementTxArray)
		for idx < lenArray {
			if feeReplacementTxArray[idx].BlkHeight+ProcessedBlkCacheDepth < curIncBlkHeight {
				feeReplacementTxArray[lenArray-1], feeReplacementTxArray[idx] = feeReplacementTxArray[idx], feeReplacementTxArray[lenArray-1]
				lenArray--
			} else {
				idx++
			}
		}
		feeReplacementTxArray = feeReplacementTxArray[:lenArray]

		// get list of processed batch IDs
		processedBatchIDs := map[string]bool{}
		for _, value := range broadcastTxArray {
			processedBatchIDs[value.BatchID] = true
		}
		for _, value := range feeReplacementTxArray {
			processedBatchIDs[value.BatchID] = true
		}
		for _, value := range confirmedTxArray {
			processedBatchIDs[value.BatchID] = true
		}

		var tempBroadcastTxArray1 []*BroadcastTx
		var tempBroadcastTxArray2 []*BroadcastTx
		tempBroadcastTxArray1, err = b.getBroadcastTxsFromBeaconHeight(processedBatchIDs, nextBlkHeight+IncBlockBatchSize-1)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Incognito block - with err: %v", err))
			return
		}
		feeReplacementTxArray, tempBroadcastTxArray2, err = b.getBroadcastReplacementTx(feeReplacementTxArray)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve get broadcast txs - with err: %v", err))
			return
		}
		tempBroadcastTxArray := append(tempBroadcastTxArray1, tempBroadcastTxArray2...)

		for _, tx := range tempBroadcastTxArray {
			err := b.broadcastTx(tx.TxContent)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not retrieve get broadcast txs - with err: %v", err))
				return
			}
		}
		broadcastTxArray = append(broadcastTxArray, tempBroadcastTxArray...)

		// check confirmed -> send rpc to notify the Inc chain
		relayingBTCHeight, err := b.getLatestBTCBlockHashFromIncog()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}
		idx = 0
		lenArray = len(broadcastTxArray)
		for idx < lenArray {
			txHash := broadcastTxArray[idx].TxHash
			isConfirmed, btcBlockHeight := b.isConfirmedBTCTx(txHash)

			if isConfirmed && btcBlockHeight <= relayingBTCHeight {
				// generate BTC proof
				btcProof, err := b.buildProof(txHash, btcBlockHeight)
				fmt.Printf("%+v\n", btcProof)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not generate BTC proof - with err: %v", err))
					return
				}

				// submit confirmed tx
				_, err = b.submitConfirmedTx(btcProof, broadcastTxArray[idx].BatchID)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not submit confirmed tx - with err: %v", err))
					return
				}
				confirmedTxArray = append(confirmedTxArray, &ConfirmedTx{
					BatchID:   broadcastTxArray[idx].BatchID,
					BlkHeight: nextBlkHeight + IncBlockBatchSize - 1,
				})
				broadcastTxArray[lenArray-1], broadcastTxArray[idx] = broadcastTxArray[idx], broadcastTxArray[lenArray-1]
				lenArray--
			} else {
				idx++
			}
		}
		broadcastTxArray = broadcastTxArray[:lenArray]

		// check if waiting too long -> send rpc to notify the Inc chain for fee replacement
		idx = 0
		lenArray = 0
		for idx < lenArray {
			if b.isTimeoutBTCTx(broadcastTxArray[idx].BlkHeight, curIncBlkHeight) { // waiting too long
				tx := broadcastTxArray[idx]
				newFee, err := utils.GetNewFee(len(tx.TxContent), tx.FeePerRequest, tx.NumOfRequests)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not get new fee - with err: %v", err))
					return
				}
				// notify the Inc chain for fee replacement
				txID, err := b.requestFeeReplacement(tx.BatchID, newFee)
				feeReplacementTxArray = append(feeReplacementTxArray, &FeeReplacementTx{
					ReqTxID: txID,
					BatchID: tx.BatchID,
				})
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not request fee replacement - with err: %v", err))
					return
				}
				broadcastTxArray[lenArray-1], broadcastTxArray[idx] = broadcastTxArray[idx], broadcastTxArray[lenArray-1]
				lenArray--
			} else {
				idx++
			}
		}
		broadcastTxArray = broadcastTxArray[:lenArray]

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
