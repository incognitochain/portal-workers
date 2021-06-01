package workers

import (
	"os"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/incognitochain/portal-workers/utxomanager"
)

func TestRelayBTCBlock(t *testing.T) {
	os.Setenv("INCOGNITO_PAYMENT_ADDRESS", "12svfkP6w5UDJDSCwqH978PvqiqBxKmUnA9em9yAYWYJVRv7wuXY1qhhYpPAm4BDz2mLbFrRmdK3yRhnTqJCZXKHUmoi7NV83HCH2YFpctHNaDdkSiQshsjw2UFUuwdEvcidgaKmF3VJpY5f8RdN")
	os.Setenv("INCOGNITO_PRIVATE_KEY", "112t8roafGgHL1rhAP9632Yef3sx5k8xgp8cwK4MCJsCL1UWcxXvpzg97N4dwvcD735iKf31Q2ZgrAvKfVjeSUEvnzKJyyJD3GqqSZdxN4or")
	os.Setenv("INCOGNITO_PROTOCOL", "http")
	os.Setenv("INCOGNITO_HOST", "127.0.0.1")
	os.Setenv("INCOGNITO_PORT", "9334")
	os.Setenv("BTC_NETWORK", "test")
	os.Setenv("BTC_NODE_HOST", "51.161.119.66")
	os.Setenv("BTC_NODE_PORT", "18443")
	os.Setenv("BTC_NODE_USERNAME", "thach")
	os.Setenv("BTC_NODE_PASSWORD", "deptrai")

	b := &BTCRelayerV2{}
	b.Init(3, "BTC Header Relayer", 60, os.Getenv("BTC_NETWORK"), nil)

	blkHeight := 1975867
	for blkHeight <= 1975877 {
		btcBlockHeight := int64(blkHeight)
		blkHash, err := b.btcClient.GetBlockHash(btcBlockHeight)
		if err != nil {
			t.FailNow()
		}
		msgBlk, err := b.btcClient.GetBlock(blkHash)
		if err != nil {
			t.FailNow()
		}
		msgBlk.Transactions = []*wire.MsgTx{}

		err = b.relayBTCBlockToIncognito(btcBlockHeight, msgBlk, utxomanager.UTXO{}, nil)
		if err != nil {
		} else {
			blkHeight++
		}
	}
}
