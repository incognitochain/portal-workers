package workers

import (
	"fmt"
	"os"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

func TestRelayBTCBlock(t *testing.T) {
	os.Setenv("INCOGNITO_PAYMENT_ADDRESS", "12svfkP6w5UDJDSCwqH978PvqiqBxKmUnA9em9yAYWYJVRv7wuXY1qhhYpPAm4BDz2mLbFrRmdK3yRhnTqJCZXKHUmoi7NV83HCH2YFpctHNaDdkSiQshsjw2UFUuwdEvcidgaKmF3VJpY5f8RdN")
	os.Setenv("INCOGNITO_PRIVATE_KEY", "112t8roafGgHL1rhAP9632Yef3sx5k8xgp8cwK4MCJsCL1UWcxXvpzg97N4dwvcD735iKf31Q2ZgrAvKfVjeSUEvnzKJyyJD3GqqSZdxN4or")
	os.Setenv("INCOGNITO_PROTOCOL", "http")
	os.Setenv("INCOGNITO_HOST", "51.79.76.38")
	os.Setenv("INCOGNITO_PORT", "8334")
	os.Setenv("BTC_NETWORK", "reg")
	os.Setenv("BTC_NODE_HOST", "51.79.76.38")
	os.Setenv("BTC_NODE_PORT", "18443")
	os.Setenv("BTC_NODE_USERNAME", "admin")
	os.Setenv("BTC_NODE_PASSWORD", "123123AZ")

	b := &BTCRelayerV2{}
	b.Init(3, "BTC Header Relayer", 60, os.Getenv("BTC_NETWORK"))

	for blkHeight := 4640; blkHeight < 5496; blkHeight++ {
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

		err = b.relayBTCBlockToIncognito(btcBlockHeight, msgBlk)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			t.FailNow()
		}
		fmt.Printf("Relayed block: %+v\n", blkHeight)
	}
}
