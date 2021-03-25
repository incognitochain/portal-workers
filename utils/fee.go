package utils

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

const (
	MAX_RATE_INCREASING_STEP     = 1.2
	DEFAULT_RATE_INCREASING_STEP = 1.15
)

type FeeAPIResponseBody struct {
	FastestFee  uint64 `json:"fastestFee"`
	HalfHourFee uint64 `json:"halfHourFee"`
	HourFee     uint64 `json:"hourFee"`
}

func GetNewFee(txSize int, oldFeePerUnshieldRequest uint, numberOfUnshieldRequest uint) (uint, error) { // pToken
	oldFee := uint64(oldFeePerUnshieldRequest) * uint64(numberOfUnshieldRequest) / 10 // oldFee: satoshi, oldFeePerUnshieldRequest: nano pBTC
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

	newFee := uint64(float64(responseBody.HalfHourFee) * float64(txSize) * 1.2)
	if newFee < oldFee {
		newFee = uint64(float64(responseBody.FastestFee) * float64(txSize) * 1.2)
	}
	if newFee < oldFee || newFee > uint64(float64(oldFee)*MAX_RATE_INCREASING_STEP) {
		newFee = uint64(float64(oldFee) * DEFAULT_RATE_INCREASING_STEP)
	}

	newFeePerUnshieldRequest := uint((newFee-1)/uint64(numberOfUnshieldRequest)+1) * 10 // newFeePerUnshieldRequest: nano pBTC

	return newFeePerUnshieldRequest, nil
}
