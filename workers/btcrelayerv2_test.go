package workers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
)

func TestBuildBlkHeader(t *testing.T) {

	b := &BTCBroadcastingManager{}
	// init bitcoin rpcclient
	connCfg := &rpcclient.ConnConfig{
		Host:         fmt.Sprintf("%s:%s", "51.161.119.66", "18443"),
		User:         "thach",
		Pass:         "deptrai",
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}

	var err error
	b.btcClient, err = rpcclient.New(connCfg, nil)
	if err != nil {
		t.FailNow()
	}

	btcBlockHeight := int64(1937581)
	blkHash, err := b.btcClient.GetBlockHash(btcBlockHeight)
	if err != nil {
		t.FailNow()
	}
	msgBlk, err := b.btcClient.GetBlock(blkHash)
	if err != nil {
		t.FailNow()
	}
	msgBlk.Transactions = []*wire.MsgTx{}
	msgBlkBytes, err := json.Marshal(msgBlk)
	if err != nil {
		t.FailNow()
	}

	headerBlockStr := base64.StdEncoding.EncodeToString(msgBlkBytes)
	t.Logf("Header: %+v", headerBlockStr)
}
