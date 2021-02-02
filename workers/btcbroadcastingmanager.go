package workers

import (
	"fmt"
	"os"

	"github.com/blockcypher/gobcy"
	"github.com/incognitochain/portal-workers/utils"
)

type BTCBroadcastingManager struct {
	WorkerAbs
	RPCUnconfirmedBroadcastTxsReader *utils.HttpClient
}

func (b *BTCBroadcastingManager) Init(id int, name string, freq int, network string) {
	b.WorkerAbs.Init(id, name, freq, network)
}

func (b *BTCBroadcastingManager) Execute() {
	b.Logger.Info("BTCBroadcastingManager agent is executing...")

	bc := gobcy.API{Token: os.Getenv("BLOCKCYPHER_TOKEN"), Coin: "btc", Chain: b.GetNetwork()}
	btcCypherChain, err := bc.GetChain()
	if err != nil {
		msg := fmt.Sprintf("Could not get btc chain info from cypher api - with err: %v", err)
		b.Logger.Error(msg)
		utils.SendSlackNotification(msg)
		return
	}

	alertMsg := fmt.Sprintf(
		"The latest block info from Cypher: height (%d), hash (%s)",
		btcCypherChain.Height, btcCypherChain.Hash,
	)
	utils.SendSlackNotification(alertMsg)
}
