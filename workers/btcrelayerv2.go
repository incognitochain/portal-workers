package workers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/incognitochain/go-incognito-sdk-v2/coin"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"

	"github.com/0xkraken/btcd/rpcclient"
	"github.com/0xkraken/btcd/wire"
)

const (
	BTCBlockBatchSize = 10 // BTCBlockBatchSize is BTC block batch size
	BlockStepBacks    = 8  // BlockStepBacks is number of blocks that the job needs to step back to solve fork situation
)

type btcBlockRes struct {
	msgBlock    *wire.MsgBlock
	blockHeight int64
	err         error
}

type BTCRelayerV2 struct {
	WorkerAbs
	btcClient *rpcclient.Client
}

func (b *BTCRelayerV2) Init(id int, name string, freq int, network string, utxoManager *utxomanager.UTXOManager) error {
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

func (b *BTCRelayerV2) relayBTCBlockToIncognito(btcBlockHeight int64, msgBlk *wire.MsgBlock) error {
	msgBlkBytes, err := json.Marshal(msgBlk)
	if err != nil {
		return err
	}
	headerBlockStr := base64.StdEncoding.EncodeToString(msgBlkBytes)

	utxos, tmpTxID, err := b.UTXOManager.GetUTXOsByAmount(os.Getenv("INCOGNITO_PRIVATE_KEY"), DefaultNetworkFee*5)
	if err != nil {
		return err
	}
	utxoCoins := []coin.PlainCoin{}
	utxoIndices := []uint64{}
	for _, utxo := range utxos {
		utxoCoins = append(utxoCoins, utxo.Coin)
		utxoIndices = append(utxoIndices, utxo.Index.Uint64())
	}

	txID, err := b.UTXOManager.IncClient.CreateAndSendPortalRelayHeaderTransaction(
		os.Getenv("INCOGNITO_PRIVATE_KEY"),
		os.Getenv("INCOGNITO_PAYMENT_ADDRESS"),
		headerBlockStr,
		uint64(btcBlockHeight),
		utxoCoins,
		utxoIndices,
	)
	if err != nil {
		b.UTXOManager.UncachedUTXOByTmpTxID(os.Getenv("INCOGNITO_PRIVATE_KEY"), tmpTxID)
		return err
	}
	b.UTXOManager.UpdateTxID(os.Getenv("INCOGNITO_PRIVATE_KEY"), tmpTxID, txID)

	b.ExportInfoLog(fmt.Sprintf("relayBTCBlockToIncognito success (%d) with TxID: %v\n", btcBlockHeight, txID))
	fmt.Printf("Relaying block %v, TxID: %v\n", btcBlockHeight, txID)
	return nil
}

func (b *BTCRelayerV2) Execute() {
	b.ExportErrorLog("BTCRelayer worker is executing...")
	// get latest BTC block from Incognito
	latestBTCBlkHeight, err := getLatestBTCHeightFromIncogWithoutFork(b.btcClient, b.RPCBTCRelayingReaders, b.Logger)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get latest btc block height from incognito chain - with err: %v", err))
		return
	}
	b.ExportInfoLog(fmt.Sprintf("Latest BTC block height: %d", latestBTCBlkHeight))

	blockQueue := make(chan btcBlockRes, BTCBlockBatchSize)
	relayingResQueue := make(chan error, BTCBlockBatchSize)

	numRoundDontInscreaseBlock := 0

	for {
		isBTCNodeAlive := getBTCFullnodeStatus(b.btcClient)
		if !isBTCNodeAlive {
			b.ExportErrorLog("Could not connect to BTC full node")
			return
		}

		prevBlockHeight := latestBTCBlkHeight

		// get latest BTC block from Incognito
		latestBTCBlkHeight, err = getLatestBTCHeightFromIncogWithoutFork(b.btcClient, b.RPCBTCRelayingReaders, b.Logger)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get latest btc block height from incognito chain - with err: %v", err))
			return
		}

		nextBlkHeight := latestBTCBlkHeight + 1

		// wait until next BTC blocks available
		var btcBestHeight int64
		for {
			btcBestHeight, err = b.btcClient.GetBlockCount()
			if err != nil {
				b.ExportErrorLog("Could not get btc best state from BTC fullnode")
				return
			}
			if int64(nextBlkHeight) <= btcBestHeight {
				break
			}
			time.Sleep(40 * time.Second)
		}
		if prevBlockHeight == latestBTCBlkHeight && nextBlkHeight < uint64(btcBestHeight) {
			numRoundDontInscreaseBlock++
		}
		if numRoundDontInscreaseBlock == 3 {
			b.ExportErrorLog("BTC header block height does not inscrease")
			b.Quit <- true
			return
		}

		var batchSize uint64
		if nextBlkHeight+uint64(BTCBlockBatchSize-1) <= uint64(btcBestHeight) { // load until the final view
			batchSize = BTCBlockBatchSize
		} else {
			batchSize = uint64(btcBestHeight) - nextBlkHeight + 1
		}

		var wg sync.WaitGroup
		for i := nextBlkHeight; i < nextBlkHeight+batchSize; i++ {
			i := i // create locals for closure below
			wg.Add(1)
			go func() {
				defer wg.Done()
				blkHash, err := b.btcClient.GetBlockHash(int64(i))
				if err != nil {
					res := btcBlockRes{msgBlock: nil, blockHeight: int64(0), err: err}
					blockQueue <- res
					return
				}

				btcMsgBlock, err := b.btcClient.GetBlock(blkHash)
				if err != nil {
					res := btcBlockRes{msgBlock: nil, blockHeight: int64(0), err: err}
					blockQueue <- res
					return
				}

				btcMsgBlock.Transactions = []*wire.MsgTx{}
				res := btcBlockRes{msgBlock: btcMsgBlock, blockHeight: int64(i), err: err}
				blockQueue <- res
			}()
		}
		wg.Wait()

		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not get list unspent UTXOs: %+v", err))
			return
		}

		// ! Could relay not in increasing order
		for i := 0; i < int(batchSize); i++ {
			btcBlkRes := <-blockQueue
			wg.Add(1)
			go func() {
				defer wg.Done()
				if btcBlkRes.err != nil {
					relayingResQueue <- btcBlkRes.err
				} else {
					//relay next BTC block to Incognito
					err := b.relayBTCBlockToIncognito(btcBlkRes.blockHeight, btcBlkRes.msgBlock)
					relayingResQueue <- err
				}
			}()
		}
		wg.Wait()

		for i := nextBlkHeight; i < nextBlkHeight+batchSize; i++ {
			relayingErr := <-relayingResQueue

			if relayingErr != nil {
				b.ExportErrorLog(fmt.Sprintf("BTC relaying error: %v\n", relayingErr))
				return
			}
		}

		time.Sleep(100 * time.Second)
	}
}
