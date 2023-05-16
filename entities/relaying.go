package entities

import (
	"github.com/0xkraken/btcd/blockchain"
)

type RelayingBlockRes struct {
	RPCBaseRes
	Result interface{}
}

type BTCRelayingBestStateRes struct {
	RPCBaseRes
	Result *blockchain.BestState
}
