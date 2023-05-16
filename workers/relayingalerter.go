package workers

import (
	"fmt"
	"time"

	"github.com/0xkraken/btcd/rpcclient"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"
)

const (
	BTCBlockDiffThreshold        = 3
	CheckRelayingIntervalSeconds = 30 * 60
)

type RelayingAlerter struct {
	WorkerAbs
	btcClient *rpcclient.Client
}

func (b *RelayingAlerter) Init(id int, name string, freq int, network string, utxoManager *utxomanager.UTXOManager) error {
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

func (b *RelayingAlerter) Execute() {
	b.ExportErrorLog("Relaying alerter worker is executing...")

	for {
		isBTCNodeAlive := getBTCFullnodeStatus(b.btcClient)
		if !isBTCNodeAlive {
			b.ExportErrorLog("Could not connect to BTC full node")
			return
		}

		// get latest BTC block from Incognito
		btcBestState, err := getBTCBestStateFromIncog(b.RPCBTCRelayingReaders)
		if err != nil {
			b.ExportErrorLog("Could not get btc best state from Incog")
			return
		}

		currentBTCBlkHeight := uint64(btcBestState.Height)
		currentBTCBlkHashStr := btcBestState.Hash.String()

		btcBestHeight, err := b.btcClient.GetBlockCount()
		if err != nil {
			b.ExportErrorLog("Could not get btc best state from BTC fullnode")
			return
		}
		btcBestHash, err := b.btcClient.GetBlockHash(btcBestHeight)
		if err != nil {
			return
		}

		diffBlock := btcBestHeight - int64(currentBTCBlkHeight)
		msg := fmt.Sprintf(
			`
            The latest block info from BTC fullnode: height (%d), hash (%s)
            The latest block info from Incognito: height (%d), hash (%s)
            Incognito's is behind %d blocks against BTC fullnode's
            `,
			btcBestHeight, btcBestHash,
			currentBTCBlkHeight, currentBTCBlkHashStr,
			diffBlock,
		)
		if diffBlock >= BTCBlockDiffThreshold {
			b.ExportErrorLog(msg)
		} else {
			b.ExportInfoLog(msg)
		}
		fmt.Printf("%v\n", msg)

		time.Sleep(CheckRelayingIntervalSeconds * time.Second)
	}
}
