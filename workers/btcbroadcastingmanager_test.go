package workers

import (
	"os"
	"testing"

	"github.com/blockcypher/gobcy"
)

func TestBuildProof(t *testing.T) {

	b := &BTCBroadcastingManager{}
	b.Network = "test3"
	b.bcy = gobcy.API{Token: os.Getenv("BLOCKCYPHER_TOKEN"), Coin: "btc", Chain: b.GetNetwork()}
	var err error
	b.bcyChain, err = b.bcy.GetChain()
	if err != nil {
		t.Logf("Could not get btc chain info from cypher api - with err: %v", err)
		return
	}

	var txHash string
	var btcBlockHeight uint64
	var btcProof string

	txHash = "9fbfc05bc9359544ff1925ea89812ed81f38353af13f83cd34439f83769c6ba4"
	btcBlockHeight = 1937581
	btcProof, err = b.buildProof(txHash, btcBlockHeight)
	if err != nil {
		t.Logf("Gen proof failed - with err: %v", err)
		return
	}
	t.Logf("Proof: %+v", btcProof)
}
