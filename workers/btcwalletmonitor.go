package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/blockcypher/gobcy"
	btcrelaying "github.com/incognitochain/incognito-chain/relaying/btc"
	"github.com/syndtr/goleveldb/leveldb"
)

type BTCWalletMonitor struct {
	WorkerAbs
	bcy      gobcy.API
	bcyChain gobcy.Blockchain
	db       *leveldb.DB
}

func (b *BTCWalletMonitor) Init(id int, name string, freq int, network string) error {
	err := b.WorkerAbs.Init(id, name, freq, network)
	// init blockcypher instance
	b.bcy = gobcy.API{Token: os.Getenv("BLOCKCYPHER_TOKEN"), Coin: "btc", Chain: b.GetNetwork()}
	b.bcyChain, err = b.bcy.GetChain()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not get btc chain info from cypher api - with err: %v", err))
		return err
	}

	// init leveldb instance
	b.db, err = leveldb.OpenFile("db/walletmonitor", nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return err
	}

	return nil
}

func (b *BTCWalletMonitor) ExportErrorLog(msg string) {
	b.WorkerAbs.ExportErrorLog(msg)
}

func (b *BTCWalletMonitor) Execute() {
	b.Logger.Info("BTCWalletMonitor worker is executing...")
	defer b.db.Close()

	lastBTCHeightTracked := uint64(0)

	// restore from db
	lastBTCHeightTrackedBytes, err := b.db.Get([]byte("LastBTCHeightTracked"), nil)
	if err == nil {
		err := json.Unmarshal(lastBTCHeightTrackedBytes, &lastBTCHeightTracked)
		if err != nil {
			panic(fmt.Sprintf("Could not load the last BTC height tracked from db - with err: %v", err))
		}
	}

	for {
		addrInfo, err := b.bcy.GetAddrFull(MultisigAddress, map[string]string{
			"after":         strconv.FormatUint(lastBTCHeightTracked, 10),
			"confirmations": strconv.FormatInt(int64(ConfirmationThreshold), 10),
		})
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve tx to multisig wallet - with err: %v", err))
			continue
		}

		for _, tx := range addrInfo.TXs {
			if b.isReceivingTx(&tx) {
				fmt.Printf("Checking tx: %v\n", tx.Hash)
				// gen proof
				proof, err := b.buildProof(tx.Hash, uint64(tx.BlockHeight))
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not build for tx: %v - with err: %v", tx.Hash, err))
					continue
				}

				// get memo, check valid
				btcTxProof, err := btcrelaying.ParseBTCProofFromB64EncodeStr(proof)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("ShieldingProof for tx %v is invalid %v\n", tx.Hash, err))
					continue
				}
				btcAttachedMsg, err := btcrelaying.ExtractAttachedMsgFromTx(btcTxProof.BTCTx)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not extract attached message from BTC tx %v proof with err: %v", tx.Hash, err))
					continue
				}

				incAddress, err := b.extractMemo(btcAttachedMsg)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not extract incognito address in memo %v from tx %v with err: %v", btcAttachedMsg, tx.Hash, err))
					continue
				}

				fmt.Printf("Found shielding request for address %v, with BTC tx %v\n", incAddress, tx.Hash)
				// send RPC
				_, err = b.submitShieldingRequest(incAddress, proof)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not send shielding request from BTC tx %v proof with err: %v", tx.Hash, err))
					continue
				}
				// update last btc block height tracked
				if uint64(tx.BlockHeight) > lastBTCHeightTracked {
					lastBTCHeightTracked = uint64(tx.BlockHeight)
				}

			}
		}

		lastBTCHeightTrackedBytes, _ := json.Marshal(&lastBTCHeightTracked)
		err = b.db.Put([]byte("LastBTCHeightTracked"), lastBTCHeightTrackedBytes, nil)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}
		time.Sleep(15 * time.Second)
	}
}
