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

type FeeAPIResponseBody struct {
	FastestFee  uint `json:"fastestFee"`
	HalfHourFee uint `json:"halfHourFee"`
	HourFee     uint `json:"hourFee"`
}

// Get fee in the current bitcoin condition (satoshi / vbyte)
func GetCurrentRelayingFee() (uint, error) {
	response, err := http.Get("https://bitcoinfees.earn.com/api/v1/fees/recommended")
	if err != nil {
		return 0, err
	}
	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	var responseBody FeeAPIResponseBody
	err = json.Unmarshal(responseData, &responseBody)
	if err != nil {
		return 0, err
	}

	return responseBody.FastestFee, nil
}

// Get new fee per request (nano pToken)
func GetNewFee(txSize int, oldFeePerUnshieldRequest uint, numberOfUnshieldRequest uint, currentRelayingFee uint) uint { // pToken
	oldFee := uint64(oldFeePerUnshieldRequest) * uint64(numberOfUnshieldRequest) / 10 // oldFee: satoshi, oldFeePerUnshieldRequest: nano pBTC
	newFee := uint64(float64(currentRelayingFee) * float64(txSize) * OVERPAYING_RATE)

	if newFee < oldFee || newFee > uint64(float64(oldFee)*DEFAULT_RATE_INCREASING_STEP) {
		newFee = uint64(float64(oldFee) * DEFAULT_RATE_INCREASING_STEP)
	}
	newFeePerUnshieldRequest := uint((newFee-1)/uint64(numberOfUnshieldRequest)+1) * 10 // newFeePerUnshieldRequest: nano pBTC

	return newFeePerUnshieldRequest
}

func IsEnoughFee(txSize int, feePerRequest uint, numberOfRequest uint, currentRelayingFee uint) bool {
	fee := uint64(feePerRequest) * uint64(numberOfRequest) / 10 // fee: satoshi, feePerRequest: nano pToken
	expectedFee := uint64(float64(currentRelayingFee) * float64(txSize) * OVERPAYING_RATE)
	return fee >= expectedFee
}
