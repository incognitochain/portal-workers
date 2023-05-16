package workers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/0xkraken/btcd/rpcclient"
	"github.com/incognitochain/portal-workers/utils"
	"github.com/incognitochain/portal-workers/utxomanager"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	PortalVaultMonitorDefaultBeaconHeight = 1427650 // the beacon height that enable portal v4
	PortalVaultMonitorDBFileDir           = "db/portalvaultmonitor"
	PortalVaultMonitorDBObjectName        = "PortalVault-LastUpdate"
	DiffAmount                            = 112569013 // 0.087822078 btc  // diff is because diff before convert vault + fee convert
	// 112458246
	AllowDiffAmount = 0.1 * 1e8
)

type PortalVaultMonitor struct {
	WorkerAbs
	btcClient  *rpcclient.Client
	bitcoinFee uint
	db         *leveldb.DB
}

type PortalVaultDBObject struct {
	LastScannedBeaconHeight uint64
	UnshieldRequests        map[string]uint64
}

func (b *PortalVaultMonitor) Init(
	id int, name string, freq int, network string, utxoManager *utxomanager.UTXOManager,
) error {
	b.WorkerAbs.Init(id, name, freq, network, utxoManager)

	var err error

	// init bitcoin rpcclient
	b.btcClient, err = utils.BuildBTCClient()
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not initialize Bitcoin RPCClient - with err: %v", err))
		return err
	}

	return nil
}

func (b *PortalVaultMonitor) ExportErrorLog(msg string) {
	b.WorkerAbs.ExportErrorLog(msg)
}

func (b *PortalVaultMonitor) ExportInfoLog(msg string) {
	b.WorkerAbs.ExportInfoLog(msg)
}

// This function will execute a worker that has 3 main tasks:
// - Broadcast a unshielding transaction to Bitcoin network
// - Check for a Bitcoin transaction is stuck or not and request RBF transaction
// - Check a broadcasted Bitcoin transaction confirmation and notify the Incognito chain
func (b *PortalVaultMonitor) Execute() {
	b.ExportErrorLog("PortalVaultMonitor worker is executing...")
	// init leveldb instance
	var err error
	b.db, err = leveldb.OpenFile(PortalVaultMonitorDBFileDir, nil)
	if err != nil {
		b.ExportErrorLog(fmt.Sprintf("Could not open leveldb storage file - with err: %v", err))
		return
	}
	defer b.db.Close()

	// restore last scaned beacon height from db
	beaconHeight := uint64(PortalVaultMonitorDefaultBeaconHeight)
	unshieldRequests := map[string]uint64{}
	portalVaultDBObject, err := b.GetPortalVaultDBObject()
	if err == nil {
		beaconHeight = portalVaultDBObject.LastScannedBeaconHeight
		unshieldRequests = portalVaultDBObject.UnshieldRequests
	}
	fmt.Printf("Beacon height from db: %v\n", beaconHeight)
	fmt.Printf("unshieldRequests from db: %v\n", unshieldRequests)

	for {
		// get lastest beacon height
		// lastestBeaconHeight := beaconHeight
		for {
			blockChainInfo, err := getIncognitoBlockChainInfo(b.RPCClient, b.Logger)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not retrieve incognito block chain info - with err: %v", err))
				return
			}
			lastestBeaconHeight := blockChainInfo.BestBlocks[-1].Height

			if beaconHeight >= lastestBeaconHeight {
				// waiting for new beacon block
				time.Sleep(40 * time.Second)
			} else {
				beaconHeight = lastestBeaconHeight
				break
			}
		}

		// for i := beaconHeight; i <= lastestBeaconHeight; i++ {
		// get portal state at beaconHeight
		portalState, err := getPortalStateFromBeaconHeight(beaconHeight, b.RPCClient, b.Logger)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve portal state from beacon block %v - with err: %v",
				beaconHeight, err))
			return
		}

		// get total amount pBTC
		pBTCAmount, err := getPTokenAmount(b.RPCClient, b.Logger, BTCID)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve pBTC amount - with err: %v", err))
			return
		}

		// get total pbtc in waiting unshielding
		pBTCAmountInWaitingUnshields := uint64(0)
		for _, wUnshield := range portalState.WaitingUnshieldRequests[BTCID] {
			pBTCAmountInWaitingUnshields += wUnshield.Amount
		}

		totalPBTCAmount := ConvertBTCIncToExternalAmount(pBTCAmount + pBTCAmountInWaitingUnshields)

		// get total utxos amount
		utxoAmountInVault := uint64(0)
		for _, utxo := range portalState.UTXOs[BTCID] {
			utxoAmountInVault += utxo.OutputAmount
		}

		// change amount from change outputs in batch txs
		changeUtxoAmountInBatchTxs := uint64(0)
		for _, batchTx := range portalState.ProcessedUnshieldRequests[BTCID] {
			utxoAmountForBacth := uint64(0)
			for _, utxo := range batchTx.Utxos {
				utxoAmountForBacth += utxo.OutputAmount
			}

			unshieldAmountInBatch := uint64(0)
			for _, unshieldID := range batchTx.UnshieldsID {
				unshieldAmount := unshieldRequests[unshieldID]
				if unshieldAmount == 0 {
					unshieldStatus, err := getRequestUnShieldingStatus(b.RPCClient, unshieldID)
					if err != nil {
						b.ExportErrorLog(fmt.Sprintf("Could not retrieve unshield status unshieldID %v - with err"+
							": %v", unshieldID, err))
						return
					}
					unshieldAmount = unshieldStatus.UnshieldAmount
					// append unshield ID and corresponding unshield amount
					unshieldRequests[unshieldID] = unshieldAmount
				}
				unshieldAmountInBatch += ConvertBTCIncToExternalAmount(unshieldAmount)
			}

			changeAmountInBatch := utxoAmountForBacth - unshieldAmountInBatch
			changeUtxoAmountInBatchTxs += changeAmountInBatch
		}

		totalBTCAmount := utxoAmountInVault + changeUtxoAmountInBatchTxs + DiffAmount

		if totalPBTCAmount > totalBTCAmount+AllowDiffAmount {
			fmt.Printf("Beacon height %v: different vault: totalPBTCAmount %v - totalBTCAmount %v - diff amount: %v\n",
				beaconHeight, totalPBTCAmount, totalBTCAmount, totalPBTCAmount-totalBTCAmount)
			// fmt.Printf("utxoAmountInVault: %v\n", utxoAmountInVault)
			// fmt.Printf("changeUtxoAmountInBatchTxs: %v\n", changeUtxoAmountInBatchTxs)
			b.ExportErrorLog(fmt.Sprintf("Beacon height %v: Different vault: totalPBTCAmount %v - totalBTCAmount %v",
				beaconHeight, totalPBTCAmount, totalBTCAmount))
			b.ExportErrorLog(fmt.Sprintf("utxoAmountInVault: %v\n", utxoAmountInVault))
			b.ExportErrorLog(fmt.Sprintf("changeUtxoAmountInBatchTxs: %v\n", changeUtxoAmountInBatchTxs))
			b.ExportErrorLog(fmt.Sprintf("pBTCAmount: %v\n", pBTCAmount))
			b.ExportErrorLog(fmt.Sprintf("pBTCAmountInWaitingUnshields: %v\n", pBTCAmountInWaitingUnshields))
		} else {
			fmt.Printf("Beacon height %v: totalPBTCAmount is the correct to totalBTCAmount\n", beaconHeight)
		}
		// remove finished unshield id from unshieldRequests
		unshieldRequests = b.RemoveFinishedUnshieldRequests(unshieldRequests)
		fmt.Printf("unshieldRequests after remove: %+v\n", unshieldRequests)
		portalObject := &PortalVaultDBObject{
			LastScannedBeaconHeight: beaconHeight,
			UnshieldRequests:        unshieldRequests,
		}
		err = b.StorePortalVaultDBObject(portalObject)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not save portal vault object to db - with err: %v", err))
			return
		}

		sleepingTime := 10
		fmt.Printf("Sleeping: %v seconds\n", sleepingTime)
		time.Sleep(time.Duration(sleepingTime) * time.Second)
	}
}

func (b *PortalVaultMonitor) StorePortalVaultDBObject(portalVaultDBObject *PortalVaultDBObject) error {
	portalVaultDBObjectBytes, _ := json.Marshal(portalVaultDBObject)
	return b.db.Put([]byte(PortalVaultMonitorDBObjectName), portalVaultDBObjectBytes, nil)
}

func (b *PortalVaultMonitor) GetPortalVaultDBObject() (*PortalVaultDBObject, error) {
	lastUpdateBytes, err := b.db.Get([]byte(PortalVaultMonitorDBObjectName), nil)
	fmt.Printf("")
	if err != nil {
		return nil, err
	}

	var portalVaultDBObject *PortalVaultDBObject
	err = json.Unmarshal(lastUpdateBytes, &portalVaultDBObject)
	if err != nil {
		return nil, err
	}
	return portalVaultDBObject, nil
}

func (b *PortalVaultMonitor) RemoveFinishedUnshieldRequests(unshieldReqs map[string]uint64) map[string]uint64 {
	for unshieldID := range unshieldReqs {
		unshieldStatus, err := getRequestUnShieldingStatus(b.RPCClient, unshieldID)
		if err == nil && unshieldStatus.Status == PortalUnshieldReqCompletedStatus {
			delete(unshieldReqs, unshieldID)
		}
	}
	return unshieldReqs
}
