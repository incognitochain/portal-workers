package workers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/blockcypher/gobcy"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/syndtr/goleveldb/leveldb"
)

const InitIncBlockBatchSize = 1000
const FirstBroadcastTxBlockHeight = 1
const TimeoutBTCFeeReplacement = 100

type BTCBroadcastingManager struct {
	WorkerAbs
	bcy      gobcy.API
	bcyChain gobcy.Blockchain
	db       *leveldb.DB
}

func (b *BTCBroadcastingManager) Init(id int, name string, freq int, network string) error {
	err := b.WorkerAbs.Init(id, name, freq, network)
	// init blockcypher instance
	b.bcy = gobcy.API{Token: os.Getenv("BLOCKCYPHER_TOKEN"), Coin: "btc", Chain: b.GetNetwork()}
	b.bcyChain, err = b.bcy.GetChain()
	if err != nil {
		msg := fmt.Sprintf("Could not get btc chain info from cypher api - with err: %v", err)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return err
	}

	// init leveldb instance
	b.db, err = leveldb.OpenFile("db", nil)
	if err != nil {
		msg := fmt.Sprintf("Could not open leveldb storage file - with err: %v", err)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return err
	}
	defer b.db.Close()

	return nil
}

type BroadcastTxsBlock struct {
	TxRawContent []string
	TxHashes     []string
	BlkHeight    uint64
	Err          error
}
type BroadcastTx struct {
	TxHash    string
	BlkHeight uint64 // height of the broadcast tx
}

type BroadcastTxArrayObject struct {
	TxArray       []*BroadcastTx
	NextBlkHeight uint64 // height of the next block need to scan in Inc chain
}

func (b *BTCBroadcastingManager) isTimeoutBTCTx(broadcastBlockHeight uint64, curBlockHeight uint64) bool {
	return curBlockHeight-broadcastBlockHeight <= TimeoutBTCFeeReplacement
}

// todo: check if confirmed
func (b *BTCBroadcastingManager) isConfirmedBTCTx(txHash string) bool {
	return false
}

func (b *BTCBroadcastingManager) getLatestBeaconHeight() (uint64, error) {
	params := []interface{}{}
	var beaconBestStateRes entities.BeaconBestStateRes
	err := b.RPCClient.RPCCall("getbeaconbeststate", params, &beaconBestStateRes)
	if err != nil {
		return 0, err
	}

	if beaconBestStateRes.RPCError != nil {
		b.Logger.Errorf("getLatestBeaconHeight: call RPC error, %v\n", beaconBestStateRes.RPCError.StackTrace)
		return 0, errors.New(beaconBestStateRes.RPCError.Message)
	}
	return beaconBestStateRes.Result.BeaconHeight, nil
}

// todo: return a list of tx raw content, tx hash and error
func (b *BTCBroadcastingManager) getBroadcastTxsFromBeaconHeight(height uint64) ([]string, []string, error) {
	return []string{}, []string{}, nil
}

func (b *BTCBroadcastingManager) broadcastTx(txContent string) error {
	skel, err := b.bcy.PushTX(txContent)
	if err != nil {
		msg := fmt.Sprintf("Could not broadcast tx to BTC chain - with err: %v \n Decoded tx: %v", err, skel)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return err
	}
	return nil
}

func (b *BTCBroadcastingManager) Execute() {
	b.Logger.Info("BTCBroadcastingManager agent is executing...")

	nextBlkHeight := uint64(FirstBroadcastTxBlockHeight)
	broadcastTxArray := []*BroadcastTx{}

	// restore from db
	lastUpdateBytes, err := b.db.Get([]byte("BTCBroadcast-LastUpdate"), nil)
	if err == nil {
		var broadcastTxsDBObject *BroadcastTxArrayObject
		json.Unmarshal(lastUpdateBytes, &broadcastTxsDBObject)
		nextBlkHeight = broadcastTxsDBObject.NextBlkHeight
		broadcastTxArray = broadcastTxsDBObject.TxArray
	} else {
		msg := fmt.Sprintf("Could not get broadcasted tx from db - with err: %v", err)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return
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
				return
			}
		}

		var IncBlockBatchSize uint64
		if nextBlkHeight+InitIncBlockBatchSize <= curIncBlkHeight { // load until the final view
			IncBlockBatchSize = InitIncBlockBatchSize
		} else {
			IncBlockBatchSize = 1
		}

		var wg sync.WaitGroup
		broadcastTxsBlockChan := make(chan *BroadcastTxsBlock, IncBlockBatchSize)
		for idx := nextBlkHeight; idx < nextBlkHeight+IncBlockBatchSize; idx++ {
			curIdx := idx
			wg.Add(1)
			go func() {
				defer wg.Done()
				txContentArray, txHashArray, err := b.getBroadcastTxsFromBeaconHeight(curIdx)
				broadcastTxsBlockChan <- &BroadcastTxsBlock{
					TxRawContent: txContentArray,
					TxHashes:     txHashArray,
					BlkHeight:    curIdx,
					Err:          err,
				}
			}()
		}
		wg.Wait()

		tempBroadcastTx := []*BroadcastTx{}
		tempTxContentArray := []string{}
		for idx := nextBlkHeight; idx < nextBlkHeight+IncBlockBatchSize; idx++ {
			broadcastTxsBlockItem := <-broadcastTxsBlockChan
			if broadcastTxsBlockItem.Err != nil {
				if broadcastTxsBlockItem.BlkHeight <= curIncBlkHeight {
					msg := fmt.Sprintf("Could not retrieve Incognito block - with err: %v", broadcastTxsBlockItem.Err)
					b.Logger.Error(msg)
					utils.SendSlackNotification(msg)
				}
				return
			}
			for i := 0; i < len(broadcastTxsBlockItem.TxHashes); i++ {
				txHash := broadcastTxsBlockItem.TxHashes[i]
				txContent := broadcastTxsBlockItem.TxRawContent[i]
				tempBroadcastTx = append(tempBroadcastTx, &BroadcastTx{TxHash: txHash, BlkHeight: broadcastTxsBlockItem.BlkHeight})
				tempTxContentArray = append(tempTxContentArray, txContent)
			}
		}

		// if there is no error
		broadcastTxArray = append(broadcastTxArray, tempBroadcastTx...)
		for _, txContent := range tempTxContentArray {
			err := b.broadcastTx(txContent)
			if err != nil {
				return
			}
		}

		// check confirmed -> send rpc to notify the Inc chain
		idx := 0
		lenArray := len(broadcastTxArray)
		for idx < lenArray {
			if b.isConfirmedBTCTx(broadcastTxArray[idx].TxHash) {
				// todo: send rpc to notify the Inc chain
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
				// todo: send rpc to notify the Inc chain for fee replacement
				broadcastTxArray[lenArray-1], broadcastTxArray[idx] = broadcastTxArray[idx], broadcastTxArray[lenArray-1]
				lenArray--
			} else {
				idx++
			}
		}

		nextBlkHeight += IncBlockBatchSize

		// update to db
		BroadcastTxArrayObjectBytes, _ := json.Marshal(&BroadcastTxArrayObject{
			TxArray:       broadcastTxArray,
			NextBlkHeight: nextBlkHeight,
		})
		err = b.db.Put([]byte("BTCBroadcast-LastUpdate"), BroadcastTxArrayObjectBytes, nil)
		if err != nil {
			msg := fmt.Sprintf("Could not save object to db - with err: %v", err)
			b.Logger.Error(msg)
			utils.SendSlackNotification(msg)
			return
		}

		time.Sleep(2 * time.Second)
	}
}
