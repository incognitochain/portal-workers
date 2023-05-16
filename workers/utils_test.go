package workers

import (
	"fmt"
	"testing"
)

func TestBroadcastTx(t *testing.T) {
	txHex := ""
	txID, err := broadcastTxToBlockStream(txHex)
	fmt.Println("txID, err: ", txID, err)
}
