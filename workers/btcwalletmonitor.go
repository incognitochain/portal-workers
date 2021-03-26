package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
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

type ShieldingInfo struct {
	IncAddress     string
	Proof          string
	BTCBlockHeight uint64
}

type ShieldingTxArrayObject struct {
	LastBTCHeightTracked uint64
	WaitingShieldingList map[string]*ShieldingInfo
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
	waitingShieldingList := map[string]*ShieldingInfo{}

	lastUpdateBytes, err := b.db.Get([]byte("BTCMonitor-LastUpdate"), nil)
	if err == nil {
		var shieldingTxArrayObject *ShieldingTxArrayObject
		json.Unmarshal(lastUpdateBytes, &shieldingTxArrayObject)
		lastBTCHeightTracked = shieldingTxArrayObject.LastBTCHeightTracked
		waitingShieldingList = shieldingTxArrayObject.WaitingShieldingList
	}

	// restore from db
	lastBTCHeightTrackedBytes, err := b.db.Get([]byte("LastBTCHeightTracked"), nil)
	if err == nil {
		err := json.Unmarshal(lastBTCHeightTrackedBytes, &lastBTCHeightTracked)
		if err != nil {
			panic(fmt.Sprintf("Could not load the last BTC height tracked from db - with err: %v", err))
		}
	}

	for {
		fmt.Printf("=== Scan tx from BTC block height: %v ===\n", lastBTCHeightTracked+1)
		addrInfo, err := b.bcy.GetAddrFull(MultisigAddress, map[string]string{
			"after":         strconv.FormatUint(lastBTCHeightTracked+1, 10),
			"confirmations": strconv.FormatInt(int64(BTCConfirmationThreshold), 10),
		})
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve tx to multisig wallet - with err: %v", err))
			continue
		}

		// check confirmed -> send rpc to notify the Inc chain
		relayingBTCHeight, err := b.getLatestBTCBlockHashFromIncog()
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve Inc relaying BTC block height - with err: %v", err))
			return
		}
		fmt.Printf("Len of waiting shielding requests: %v\n", len(waitingShieldingList))

		for _, tx := range addrInfo.TXs {
			time.Sleep(1 * time.Second)
			// update last btc block height tracked
			if tx.BlockHeight > 0 && uint64(tx.BlockHeight) > lastBTCHeightTracked {
				lastBTCHeightTracked = uint64(tx.BlockHeight)
			}

			if b.isReceivingTx(&tx) {
				fmt.Printf("Checking tx %v from height %v\n", tx.Hash, tx.BlockHeight)
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
				waitingShieldingList[tx.Hash] = &ShieldingInfo{
					IncAddress:     incAddress,
					Proof:          proof,
					BTCBlockHeight: uint64(tx.BlockHeight),
				}
			}
		}

		sentShieldingRequest := make(chan string, len(waitingShieldingList))
		var wg sync.WaitGroup
		for txHash, value := range waitingShieldingList {
			if value.BTCBlockHeight+BTCConfirmationThreshold <= relayingBTCHeight*100 {
				// send RPC
				txID, err := b.submitShieldingRequest(value.IncAddress, value.Proof)
				if err != nil {
					b.ExportErrorLog(fmt.Sprintf("Could not send shielding request from BTC tx %v proof with err: %v", txHash, err))
					continue
				}
				fmt.Printf("Shielding txID: %v\n", txID)
				wg.Add(1)
				go func() {
					defer wg.Done()
					status, err := b.getRequestShieldingStatus(txID)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not get request shielding status from BTC tx %v - with err: %v", txHash, err))
					} else {
						if status == 0 { // rejected
							b.ExportErrorLog(fmt.Sprintf("Request shielding failed BTC tx %v, shielding txID %v", txHash, txID))
						}
						sentShieldingRequest <- txHash
					}
				}()
			}
		}
		wg.Wait()

		close(sentShieldingRequest)
		for txHash := range sentShieldingRequest {
			delete(waitingShieldingList, txHash)
		}

		shieldingTxArrayObjectBytes, _ := json.Marshal(&ShieldingTxArrayObject{
			LastBTCHeightTracked: lastBTCHeightTracked,
			WaitingShieldingList: waitingShieldingList,
		})
		err = b.db.Put([]byte("BTCMonitor-LastUpdate"), shieldingTxArrayObjectBytes, nil)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save object to db - with err: %v", err))
			return
		}
		time.Sleep(15 * time.Second)
	}
}
