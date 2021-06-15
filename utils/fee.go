package utils

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

const (
	OVERPAYING_RATE              = 1.1
	DEFAULT_RATE_INCREASING_STEP = 1.15
)

type BlockCypherFeeResponse struct {
	HighFee   uint `json:"high_fee_per_kb"`
	MediumFee uint `json:"medium_fee_per_kb"`
	LowFee    uint `json:"low_fee_per_kb"`
}

// Get fee in the current bitcoin condition (satoshi / vbyte)
func GetCurrentRelayingFee() (uint, error) {
	response, err := http.Get("https://api.blockcypher.com/v1/btc/main")
	if err != nil {
		return 0, err
	}
	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	var responseBody BlockCypherFeeResponse
	err = json.Unmarshal(responseData, &responseBody)
	if err != nil {
		return 0, err
	}
	feePerVByte := float64(responseBody.MediumFee) / 1024

	return uint(feePerVByte), nil
}

// Get new fee per request (nano pToken)
func GetNewFee(vsize int, oldFeePerUnshieldRequest uint, numberOfUnshieldRequest uint, currentRelayingFee uint) uint { // pToken
	oldFee := uint64(oldFeePerUnshieldRequest) * uint64(numberOfUnshieldRequest) / 10 // oldFee: satoshi, oldFeePerUnshieldRequest: nano pBTC
	newFee := uint64(float64(currentRelayingFee) * float64(vsize) * OVERPAYING_RATE)

	if newFee < oldFee || newFee > uint64(float64(oldFee)*DEFAULT_RATE_INCREASING_STEP) {
		newFee = uint64(float64(oldFee) * DEFAULT_RATE_INCREASING_STEP)
	}
	newFeePerUnshieldRequest := uint((newFee-1)/uint64(numberOfUnshieldRequest)+1) * 10 // newFeePerUnshieldRequest: nano pBTC

	return newFeePerUnshieldRequest
}

func IsEnoughFee(vsize int, feePerRequest uint, numberOfRequest uint, currentRelayingFee uint) bool {
	fee := uint64(feePerRequest) * uint64(numberOfRequest) / 10 // fee: satoshi, feePerRequest: nano pToken
	expectedFee := uint64(float64(currentRelayingFee) * float64(vsize) * OVERPAYING_RATE)
	return fee >= expectedFee
}
