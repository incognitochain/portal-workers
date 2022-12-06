package workers

import (
	"fmt"
	"testing"

	"github.com/incognitochain/portal-workers/utils"
)

func TestCallPortalVault(t *testing.T) {
	beaconHeight := uint64(2551081)

	rpcClient := utils.NewHttpClient("https://mainnet.incognito.org/fullnode", "", "", "")
	btcID := "b832e5d3b1f01a4f0623f7fe91d6673461e1f5d37d91fe78c5c2e6183ff39696"

	// rpcClient := utils.NewHttpClient("https://testnet.incognito.org/fullnode", "", "", "")
	// btcID := "4584d5e9b2fc0337dfb17f4b5bb025e5b82c38cfa4f54e8a3d4fcdd03954ff82"
	portalState, err := GetPortalState(beaconHeight, rpcClient)
	if err != nil {
		fmt.Printf("Err: %v\n", err)
	}

	totalAmount := uint64(0)
	maxAmount := uint64(0)
	count1 := 0
	count2 := 0
	count3 := 0
	count4 := 0
	for _, utxo := range portalState.UTXOs[btcID] {
		totalAmount += utxo.OutputAmount
		if utxo.OutputAmount >= 0.2*1e8 {
			count1++
		} else if utxo.OutputAmount >= 0.1*1e8 {
			count2++
		} else if utxo.OutputAmount >= 0.01*1e8 {
			count3++
		} else {
			count4++
		}

		if maxAmount < utxo.OutputAmount {
			maxAmount = utxo.OutputAmount
		}

	}
	fmt.Printf("Beacon height %v - Total amount utxos: %v - Len utxos %v\n", beaconHeight, totalAmount,
		len(portalState.UTXOs[btcID]))
	fmt.Printf("maxAmount: %v\n", maxAmount)
	fmt.Printf("count1: %v\n", count1)
	fmt.Printf("count2: %v\n", count2)
	fmt.Printf("count3: %v\n", count3)
	fmt.Printf("count4: %v\n", count4)
}

type UnshieldInfo struct {
	ExternalFee        uint64
	NumUnshieldInBatch int
	NumUTXOs           int
}

func TestGetUnshieldFee(t *testing.T) {
	fromBeaconHeight := uint64(2391000)
	// toBeaconHeight := uint64(2390720)
	toBeaconHeight := uint64(2520290)

	rpcClient := utils.NewHttpClient("https://mainnet.incognito.org/fullnode", "", "", "")
	btcID := "b832e5d3b1f01a4f0623f7fe91d6673461e1f5d37d91fe78c5c2e6183ff39696"
	// rpcClient := utils.NewHttpClient("https://testnet.incognito.org/fullnode", "", "", "")
	// btcID := "4584d5e9b2fc0337dfb17f4b5bb025e5b82c38cfa4f54e8a3d4fcdd03954ff82"

	unshieldReqs := map[string]UnshieldInfo{}
	for blk := fromBeaconHeight; blk <= toBeaconHeight; blk+=30 {
		portalState, err := GetPortalState(blk, rpcClient)
		if err != nil {
			fmt.Printf("Err get portal state: %v\n", err)
			return
		}

		// get processed batch
		batchs := portalState.ProcessedUnshieldRequests[btcID]
		// if len(batchs) == 0 {
		// 	fmt.Println("Batch empty!")
		// }
		for _, batch := range batchs {
			unshieldIDs := batch.UnshieldsID
			numUnshieldInBatch := len(unshieldIDs)
			numUXTOs := len(batch.Utxos)

			for _, unshieldID := range unshieldIDs {
				if _, ok := unshieldReqs[unshieldID]; ok {
					continue
				}

				// get unshield status
				unshieldStatus, err := getRequestUnShieldingStatus(rpcClient, unshieldID)
				if err != nil || unshieldStatus.Status != 2 {
					fmt.Printf("Err get unshield ID %v: %v\n", unshieldID, err)
					return
				}

				unshieldReqs[unshieldID] = UnshieldInfo{
					ExternalFee:        unshieldStatus.ExternalFee,
					NumUnshieldInBatch: numUnshieldInBatch,
					NumUTXOs:           numUXTOs,
				}

				fmt.Printf("AAA,%v,%v,%v\n", unshieldStatus.ExternalFee, numUnshieldInBatch, numUXTOs)
			}
		}
	}
}
