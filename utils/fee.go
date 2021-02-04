package utils

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type FeeAPIResponseBody struct {
	FastestFee  uint64 `json:"fastestFee"`
	HalfHourFee uint64 `json:"halfHourFee"`
	HourFee     uint64 `json:"hourFee"`
}

func GetNewFee(txSize int, oldFee uint64) (uint64, error) {
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

	newFee := responseBody.HalfHourFee * uint64(txSize)
	if newFee < oldFee {
		newFee = oldFee
	}
	return newFee, nil
}
