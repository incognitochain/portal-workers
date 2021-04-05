package utils

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/rpcclient"
)

func BuildBTCClient() (*rpcclient.Client, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", os.Getenv("BTC_NODE_HOST"), os.Getenv("BTC_NODE_PORT")),
		User:         os.Getenv("BTC_NODE_USERNAME"),
		Pass:         os.Getenv("BTC_NODE_PASSWORD"),
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}
	return rpcclient.New(connCfg, nil)
}
