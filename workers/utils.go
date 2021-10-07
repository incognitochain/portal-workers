package workers

import (
	"sync"
	"time"
)

var feePerVByte float64 // satoshi / byte
var feeRWLock sync.RWMutex

type BlockCypherFeeResponse struct {
	HighFee   uint `json:"high_fee_per_kb"`
	MediumFee uint `json:"medium_fee_per_kb"`
	LowFee    uint `json:"low_fee_per_kb"`
}

func getCurrentRelayingFee() {
	for {
		func() {
			feeRWLock.Lock()
			defer func() {
				feeRWLock.Unlock()
				time.Sleep(3 * time.Minute)
			}()
			feePerVByte = 10
		}()
	}
}
