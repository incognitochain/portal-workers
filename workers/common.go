package workers

import "github.com/btcsuite/btcd/rpcclient"

func getBTCFullnodeStatus(btcClient *rpcclient.Client) bool {
	_, err := btcClient.GetBlockChainInfo()
	return err == nil
}
