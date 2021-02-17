package workers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/blockcypher/gobcy"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/syndtr/goleveldb/leveldb"
)

const InitIncBlockBatchSize = 1000
const FirstBroadcastTxBlockHeight = 1
const TimeoutBTCFeeReplacement = 100
const ConfirmationThreshold = 6

type BTCBroadcastingManager struct {
	WorkerAbs
	bcy      gobcy.API
	bcyChain gobcy.Blockchain
	db       *leveldb.DB
}

type BroadcastTx struct {
	TxContent string
	TxHash    string
	BatchID   int
	BlkHeight uint64 // height of the broadcast tx
}

type BroadcastTxsBlock struct {
	TxArray   []*BroadcastTx
	BlkHeight uint64
	Err       error
}

type BroadcastTxArrayObject struct {
	TxArray       []*BroadcastTx
	NextBlkHeight uint64 // height of the next block need to scan in Inc chain
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

func (b *BTCBroadcastingManager) isTimeoutBTCTx(broadcastBlockHeight uint64, curBlockHeight uint64) bool {
	return curBlockHeight-broadcastBlockHeight <= TimeoutBTCFeeReplacement
}

// return boolean value of transaction confirmation and bitcoin block height
func (b *BTCBroadcastingManager) isConfirmedBTCTx(txHash string) (bool, uint64) {
	tx, err := b.bcy.GetTX(txHash, nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not check the confirmation of tx in BTC chain - with err: %v \n Tx hash: %v", err, txHash))
		return false, 0
	}
	return tx.Confirmations >= ConfirmationThreshold, uint64(tx.BlockHeight)
}

func (b *BTCBroadcastingManager) broadcastTx(txContent string) error {
	skel, err := b.bcy.PushTX(txContent)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not broadcast tx to BTC chain - with err: %v \n Decoded tx: %v", err, skel))
		return err
	}
	return nil
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

func (b *BTCBroadcastingManager) getLatestBTCBlockHashFromIncog() (uint64, error) {
	params := []interface{}{}
	var btcRelayingBestStateRes entities.BTCRelayingBestStateRes
	err := b.RPCClient.RPCCall("getbtcrelayingbeststate", params, &btcRelayingBestStateRes)
	if err != nil {
		return 0, err
	}
	if btcRelayingBestStateRes.RPCError != nil {
		b.Logger.Errorf("getLatestBTCBlockHashFromIncog: call RPC error, %v\n", btcRelayingBestStateRes.RPCError.StackTrace)
		return 0, errors.New(btcRelayingBestStateRes.RPCError.Message)
	}

	// check whether there was a fork happened or not
	btcBestState := btcRelayingBestStateRes.Result
	if btcBestState == nil {
		return 0, errors.New("BTC relaying best state is nil")
	}
	currentBTCBlkHeight := btcBestState.Height
	return uint64(currentBTCBlkHeight), nil
}

func (b *BTCBroadcastingManager) getBroadcastTxsFromBeaconHeight(height uint64) *BroadcastTxsBlock {
	params := []interface{}{
		height,
	}
	var beaconblockRes entities.BeaconBlockByHeightRes
	err := b.RPCClient.RPCCall("retrievebeaconblockbyheight", params, &beaconblockRes)
	if err != nil {
		return &BroadcastTxsBlock{
			TxArray:   []*BroadcastTx{},
			BlkHeight: height,
			Err:       err,
		}
	}
	if beaconblockRes.RPCError != nil {
		b.Logger.Errorf("getBroadcastTxsFromBeaconHeight: call RPC error, %v\n", beaconblockRes.RPCError.StackTrace)
		return &BroadcastTxsBlock{
			TxArray:   []*BroadcastTx{},
			BlkHeight: height,
			Err:       errors.New(beaconblockRes.RPCError.Message),
		}
	}

	// todo: get tx raw content, hash, batchID
	for _, instruction := range beaconblockRes.Result[0].Instructions {
		fmt.Println(instruction)
	}

	return &BroadcastTxsBlock{
		TxArray:   []*BroadcastTx{},
		BlkHeight: height,
		Err:       nil,
	}
}

func (b *BTCBroadcastingManager) buildProof(txID string, blkHeight uint64) (string, error) {
	cypherBlock, err := b.bcy.GetBlock(
		int(blkHeight),
		"",
		map[string]string{
			"txstart": "0",
			"limit":   "500",
		},
	)

	if err != nil {
		return "", err
	}

	txIDs := cypherBlock.TXids
	txHashes := make([]*chainhash.Hash, len(txIDs))
	for i := 0; i < len(txIDs); i++ {
		txHashes[i], _ = chainhash.NewHashFromStr(txIDs[i])
	}

	msgTx := utils.BuildMsgTxFromCypher(txID, b.GetNetwork())
	txHash := msgTx.TxHash()
	blkHash, _ := chainhash.NewHashFromStr(cypherBlock.Hash)

	merkleProofs := utils.BuildMerkleProof(txHashes, &txHash)
	btcProof := utils.BTCProof{
		MerkleProofs: merkleProofs,
		BTCTx:        msgTx,
		BlockHash:    blkHash,
	}
	btcProofBytes, _ := json.Marshal(btcProof)
	btcProofStr := base64.StdEncoding.EncodeToString(btcProofBytes)

	return btcProofStr, nil
}

func (b *BTCBroadcastingManager) Execute() {
	b.Logger.Info("BTCBroadcastingManager agent is executing...")
	defer b.db.Close()

	nextBlkHeight := uint64(FirstBroadcastTxBlockHeight)
	broadcastTxArray := []*BroadcastTx{}

	// restore from db
	lastUpdateBytes, err := b.db.Get([]byte("BTCBroadcast-LastUpdate"), nil)
	if err == nil {
		var broadcastTxsDBObject *BroadcastTxArrayObject
		json.Unmarshal(lastUpdateBytes, &broadcastTxsDBObject)
		nextBlkHeight = broadcastTxsDBObject.NextBlkHeight
		broadcastTxArray = broadcastTxsDBObject.TxArray
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

		var wg sync.WaitGroup
		broadcastTxsBlockChan := make(chan *BroadcastTxsBlock, IncBlockBatchSize)
		for idx := nextBlkHeight; idx < nextBlkHeight+IncBlockBatchSize; idx++ {
			curIdx := idx
			wg.Add(1)
			go func() {
				defer wg.Done()
				broadcastTxsBlockChan <- b.getBroadcastTxsFromBeaconHeight(curIdx)
			}()
		}
		wg.Wait()

		tempBroadcastTx := []*BroadcastTx{}
		for idx := nextBlkHeight; idx < nextBlkHeight+IncBlockBatchSize; idx++ {
			broadcastTxsBlockItem := <-broadcastTxsBlockChan
			if broadcastTxsBlockItem.Err != nil {
				if broadcastTxsBlockItem.BlkHeight <= curIncBlkHeight {
					b.ExportErrorLog(fmt.Sprintf("Could not retrieve Incognito block - with err: %v", broadcastTxsBlockItem.Err))
				}
				return
			}
			tempBroadcastTx = append(tempBroadcastTx, broadcastTxsBlockItem.TxArray...)
		}

		// if there is no error
		broadcastTxArray = append(broadcastTxArray, tempBroadcastTx...)
		for _, tx := range tempBroadcastTx {
			err := b.broadcastTx(tx.TxContent)
			if err != nil {
				return
			}
		}

		// check confirmed -> send rpc to notify the Inc chain
		relayingBTCHeight, err := b.getLatestBTCBlockHashFromIncog()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}
		idx := 0
		lenArray := len(broadcastTxArray)
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
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}

		time.Sleep(2 * time.Second)
	}
}
