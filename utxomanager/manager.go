package utxomanager

import (
	"sync"

	go_incognito "github.com/inc-backend/go-incognito"
	"github.com/incognitochain/portal-workers/utils"
)

type UTXO struct {
	KeyImage string
	Amount   uint64
}

type UTXOManager struct {
	Unspent   map[string][]UTXO            // public key: UTXO
	Caches    map[string]map[string][]UTXO // public key: txID: UTXO
	mux       sync.Mutex
	TmpIdx    int
	Wallet    *go_incognito.Wallet
	RPCClient *utils.HttpClient
}

func NewUTXOManager(w *go_incognito.Wallet, rpcClient *utils.HttpClient) *UTXOManager {
	return &UTXOManager{
		Unspent:   map[string][]UTXO{},
		Caches:    map[string]map[string][]UTXO{},
		TmpIdx:    0,
		Wallet:    w,
		RPCClient: rpcClient,
	}
}
