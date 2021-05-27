package workers

import (
	"fmt"
	"time"
)

const (
	FirstMonitoringBlkHeight        = 1
	UnshieldingBatchTimeoutBlk      = 200
	CheckUnshieldingIntervalSeconds = 30 * 60
)

type UnshieldingAlerter struct {
	WorkerAbs
}

func (b *UnshieldingAlerter) Init(id int, name string, freq int, network string) error {
	b.WorkerAbs.Init(id, name, freq, network)

	return nil
}

func (b *UnshieldingAlerter) Execute() {
	b.ExportErrorLog("Unshielding alerter worker is executing...")

	nextBlkHeight := uint64(FirstMonitoringBlkHeight)
	firstAppearBatch := map[string]uint64{} // batchID : Incognito block height

	for {
		// wait until next blocks available
		var curIncBlkHeight uint64
		var err error
		for {
			curIncBlkHeight, err = getLatestBeaconHeight(b.RPCClient, b.Logger)
			if err != nil {
				b.ExportErrorLog(fmt.Sprintf("Could not get latest beacon height - with err: %v", err))
				return
			}
			if nextBlkHeight < curIncBlkHeight {
				break
			}
			time.Sleep(40 * time.Second)
		}

		var scannedBlkHeight uint64
		if nextBlkHeight+UnshieldingBatchTimeoutBlk <= curIncBlkHeight { // load until the final view
			scannedBlkHeight = nextBlkHeight + UnshieldingBatchTimeoutBlk - 1
		} else {
			scannedBlkHeight = curIncBlkHeight - 1
		}

		batchIDs, err := getBatchIDsFromBeaconHeight(scannedBlkHeight, b.RPCClient, b.Logger)
		if err != nil {
			b.ExportErrorLog(fmt.Sprintf("Could not retrieve batches from beacon block %v - with err: %v", scannedBlkHeight, err))
			return
		}

		newBatchInfo := map[string]uint64{}
		for _, batchID := range batchIDs {
			_, isExisted := firstAppearBatch[batchID]
			if isExisted {
				if scannedBlkHeight-firstAppearBatch[batchID]+1 >= UnshieldingBatchTimeoutBlk {
					msg := fmt.Sprintf(
						"Batch %v exists in more than %v blocks, last checked block: %v",
						batchID, scannedBlkHeight-firstAppearBatch[batchID]+1, scannedBlkHeight,
					)
					b.ExportErrorLog(msg)
					newBatchInfo[batchID] = scannedBlkHeight
				} else {
					newBatchInfo[batchID] = firstAppearBatch[batchID]
				}
			} else {
				newBatchInfo[batchID] = scannedBlkHeight
			}
		}
		firstAppearBatch = newBatchInfo

		msg := fmt.Sprintf("Incognito block height %v has %v batches", scannedBlkHeight, len(batchIDs))
		b.ExportInfoLog(msg)
		fmt.Printf("%v\n", msg)

		nextBlkHeight = scannedBlkHeight + 1
		time.Sleep(CheckUnshieldingIntervalSeconds * time.Second)
	}
}
