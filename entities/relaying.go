package entities

import (
	"github.com/btcsuite/btcd/blockchain"
)

type RelayingBlockRes struct {
	RPCBaseRes
	Result interface{}
}

type BTCRelayingBestStateRes struct {
	RPCBaseRes
	Result *blockchain.BestState
}
