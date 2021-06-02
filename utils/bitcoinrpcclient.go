package utils

import (
	"os"

	"github.com/btcsuite/btcd/rpcclient"
)

func BuildBTCClient() (*rpcclient.Client, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         os.Getenv("BTC_NODE_HOST"),
		User:         os.Getenv("BTC_NODE_USERNAME"),
		Pass:         os.Getenv("BTC_NODE_PASSWORD"),
		HTTPPostMode: true,                                     // Bitcoin core only supports HTTP POST mode
		DisableTLS:   !(os.Getenv("BTC_NODE_HTTPS") == "true"), // Bitcoin core does not provide TLS by default
	}
	return rpcclient.New(connCfg, nil)
}
