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
	RPCUnconfirmedBroadcastTxsReader *utils.HttpClient
}

func (b *BTCBroadcastingManager) Init(id int, name string, freq int, network string) {
	b.WorkerAbs.Init(id, name, freq, network)
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
	TxArray   []*BroadcastTx
	BlkHeight uint64 // height of the last updated Inc beacon block
}

func isTimeoutBTCTx(broadcastBlockHeight uint64, curBlockHeight uint64) bool {
	return curBlockHeight-broadcastBlockHeight <= TimeoutBTCFeeReplacement
}

// todo: check if confirmed
func isConfirmedBTCTx(txHash string, chain gobcy.Blockchain) bool {
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

// todo: broadcast to BTC chain
func (b *BTCBroadcastingManager) broadcastTx(txContent string) error {
	return nil
}

func (b *BTCBroadcastingManager) Execute() {
	b.Logger.Info("BTCBroadcastingManager agent is executing...")

	// init blockcypher instance
	bc := gobcy.API{Token: os.Getenv("BLOCKCYPHER_TOKEN"), Coin: "btc", Chain: b.GetNetwork()}
	btcCypherChain, err := bc.GetChain()
	if err != nil {
		msg := fmt.Sprintf("Could not get btc chain info from cypher api - with err: %v", err)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return
	}
	alertMsg := fmt.Sprintf("The latest block info from Cypher: height (%d), hash (%s)", btcCypherChain.Height, btcCypherChain.Hash)
	utils.SendSlackNotification(alertMsg)

	// init leveldb instance
	db, err := leveldb.OpenFile("db", nil)
	if err != nil {
		msg := fmt.Sprintf("Could not open leveldb storage file - with err: %v", err)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return
	}
	defer db.Close()

	lastUpdatedIncBlockHeight := uint64(FirstBroadcastTxBlockHeight) - 1
	broadcastTxArray := []*BroadcastTx{}

	// restore from db
	lastUpdateBytes, err := db.Get([]byte("BTCBroadcast-LastUpdate"), nil)
	if err == nil {
		var broadcastTxsDBObject *BroadcastTxArrayObject
		json.Unmarshal(lastUpdateBytes, &broadcastTxsDBObject)
		lastUpdatedIncBlockHeight = broadcastTxsDBObject.BlkHeight
		broadcastTxArray = broadcastTxsDBObject.TxArray
	} else {
		msg := fmt.Sprintf("Could not get broadcasted tx from db - with err: %v", err)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return
	}

	nextBlkHeight := lastUpdatedIncBlockHeight + 1
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
		broadcastTxsChan := make(chan *BroadcastTxsBlock, IncBlockBatchSize)
		for idx := nextBlkHeight; idx < nextBlkHeight+IncBlockBatchSize; idx++ {
			curIdx := idx
			wg.Add(1)
			go func() {
				defer wg.Done()
				txContentArray, txHashArray, err := b.getBroadcastTxsFromBeaconHeight(curIdx)
				broadcastTxsChan <- &BroadcastTxsBlock{
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
			broadcastTxsRes := <-broadcastTxsChan
			if broadcastTxsRes.Err != nil {
				if broadcastTxsRes.BlkHeight <= curIncBlkHeight {
					msg := fmt.Sprintf("Could not retrieve Incognito block - with err: %v", broadcastTxsRes.Err)
					b.Logger.Error(msg)
					utils.SendSlackNotification(msg)
				}
				return
			}
			for i := 0; i < len(broadcastTxsRes.TxHashes); i++ {
				txHash := broadcastTxsRes.TxHashes[i]
				txContent := broadcastTxsRes.TxRawContent[i]
				tempBroadcastTx = append(tempBroadcastTx, &BroadcastTx{TxHash: txHash, BlkHeight: broadcastTxsRes.BlkHeight})
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
			if isConfirmedBTCTx(broadcastTxArray[idx].TxHash, btcCypherChain) {
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
			if isTimeoutBTCTx(broadcastTxArray[idx].BlkHeight, curIncBlkHeight) { // waiting too long
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
			TxArray:   broadcastTxArray,
			BlkHeight: nextBlkHeight - 1,
		})
		err = db.Put([]byte("BTCBroadcast-LastUpdate"), BroadcastTxArrayObjectBytes, nil)
		if err != nil {
			msg := fmt.Sprintf("Could not save object to db - with err: %v", err)
			b.Logger.Error(msg)
			utils.SendSlackNotification(msg)
			return
		}

		time.Sleep(2 * time.Second)
	}
}
