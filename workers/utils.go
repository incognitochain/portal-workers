package workers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
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
			response, err := http.Get("https://api.blockcypher.com/v1/btc/main")
			feeRWLock.Lock()
			defer func() {
				feeRWLock.Unlock()
				time.Sleep(3 * time.Minute)
			}()
			if err != nil {
				feePerVByte = -1
				return
			}
			if response.StatusCode != 200 {
				feePerVByte = -1
				return
			}
			responseData, err := ioutil.ReadAll(response.Body)
			if err != nil {
				feePerVByte = -1
				return
			}
			var responseBody BlockCypherFeeResponse
			err = json.Unmarshal(responseData, &responseBody)
			if err != nil {
				feePerVByte = -1
				return
			}
			feePerVByte = float64(responseBody.MediumFee) / 1024
		}()
	}
}
