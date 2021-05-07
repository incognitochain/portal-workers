package workers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/incognitochain/portal-workers/entities"
	"github.com/incognitochain/portal-workers/utils"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/inc-backend/go-incognito/publish/transformer"
)

// BTCBlockBatchSize is BTC block batch size
const BTCBlockBatchSize = 1

// BlockStepBacks is number of blocks that the job needs to step back to solve fork situation
const BlockStepBacks = 8

type btcBlockRes struct {
	msgBlock    *wire.MsgBlock
	blockHeight int64
	err         error
}

type BTCRelayerV2 struct {
	WorkerAbs
	RelayingHeader *go_incognito.RelayingChainHeader
	btcClient      *rpcclient.Client
}

func (b *BTCRelayerV2) Init(id int, name string, freq int, network string) error {
	b.WorkerAbs.Init(id, name, freq, network)

	b.RelayingHeader = go_incognito.NewRelayingChainHeader(b.Client)

	var err error
	// init bitcoin rpcclient
	b.btcClient, err = utils.BuildBTCClient()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not initialize Bitcoin RPCClient - with err: %v", err))
		return err
	}

	return nil
}

func (b *BTCRelayerV2) relayBTCBlockToIncognito(
	btcBlockHeight int64,
	msgBlk *wire.MsgBlock,
) error {
	msgBlkBytes, err := json.Marshal(msgBlk)
	if err != nil {
		return err
	}
	headerBlockStr := base64.StdEncoding.EncodeToString(msgBlkBytes)

	metadata := map[string]interface{}{
		"SenderAddress": os.Getenv("INCOGNITO_PAYMENT_ADDRESS"),
		"Header":        headerBlockStr,
		"BlockHeight":   fmt.Sprintf("%v", btcBlockHeight),
	}

	result, err := b.RelayingHeader.RelayBTCHeader(os.Getenv("INCOGNITO_PRIVATE_KEY"), metadata)
	if err != nil {
		return err
	}
	resp, err := b.Client.SubmitRawData(result)
	if err != nil {
		return err
	}

	txID, err := transformer.TransformersTxHash(resp)
	if err != nil {
		return err
	}

	b.ExportInfoLog(fmt.Sprintf("relayBTCBlockToIncognito success (%d) with TxID: %v\n", btcBlockHeight, txID))
	return nil
}

func (b *BTCRelayerV2) getLatestBTCBlockHashFromIncog(btcClient *rpcclient.Client) (int32, error) {
	params := []interface{}{}
	var btcRelayingBestStateRes entities.BTCRelayingBestStateRes
	err := b.RPCClient.RPCCall("getbtcrelayingbeststate", params, &btcRelayingBestStateRes)
	if err != nil {
		return 0, err
	}
	if btcRelayingBestStateRes.RPCError != nil {
		return 0, errors.New(btcRelayingBestStateRes.RPCError.Message)
	}

	// check whether there was a fork happened or not
	btcBestState := btcRelayingBestStateRes.Result
	if btcBestState == nil {
		return 0, errors.New("BTC relaying best state is nil")
	}
	currentBTCBlkHashStr := btcBestState.Hash.String()
	currentBTCBlkHeight := btcBestState.Height
	blkHash, err := btcClient.GetBlockHash(int64(currentBTCBlkHeight))
	if err != nil {
		return 0, err
	}

	if blkHash.String() != currentBTCBlkHashStr { // fork detected
		msg := fmt.Sprintf("There was a fork happened at block %d, stepping back %d blocks now...", currentBTCBlkHeight, BlockStepBacks)
		b.Logger.Warnf(msg)
		utils.SendSlackNotification(msg)
		return currentBTCBlkHeight - BlockStepBacks, nil
	}
	return currentBTCBlkHeight, nil
}

func (b *BTCRelayerV2) Execute() {
	// get latest BTC block from Incognito
	latestBTCBlkHeight, err := b.getLatestBTCBlockHashFromIncog(b.btcClient)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get latest btc block height from incognito chain - with err: %v", err))
		return
	}
	b.ExportInfoLog(fmt.Sprintf("Latest BTC block height: %d", latestBTCBlkHeight))

	nextBlkHeight := latestBTCBlkHeight + 1
	blockQueue := make(chan btcBlockRes, BTCBlockBatchSize)
	relayingResQueue := make(chan error, BTCBlockBatchSize)
	for {
		var wg sync.WaitGroup
		for i := nextBlkHeight; i < nextBlkHeight+BTCBlockBatchSize; i++ {
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

		for i := nextBlkHeight; i < nextBlkHeight+BTCBlockBatchSize; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				btcBlkRes := <-blockQueue
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

		for i := nextBlkHeight; i < nextBlkHeight+BTCBlockBatchSize; i++ {
			relayingErr := <-relayingResQueue

			if relayingErr != nil {
				if !strings.Contains(relayingErr.Error(), "Block height out of range") {
					b.ExportErrorLog(fmt.Sprintf("BTC relaying error: %v\n", relayingErr))
				}
				return
			}
		}

		nextBlkHeight += BTCBlockBatchSize
		time.Sleep(2 * time.Second)
	}
}
