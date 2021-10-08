package utils

import (
	"os"
	"strconv"
)

const (
	DEFAULT_RATE_INCREASING_STEP = 1.09
)

// Get new fee per request (nano pToken)
func GetNewFee(vsize int, oldFeePerUnshieldRequest uint, numberOfUnshieldRequest uint, currentRelayingFee uint) uint { // pToken
	overpayingFeePercentStr := os.Getenv("OVERPAYING_FEE_PERCENT")
	overpayingFeePercent, _ := strconv.Atoi(overpayingFeePercentStr)
	overpayingRate := 1 + float64(overpayingFeePercent)/100

	oldFee := uint64(oldFeePerUnshieldRequest) * uint64(numberOfUnshieldRequest) / 10 // oldFee: satoshi, oldFeePerUnshieldRequest: nano pBTC
	newFee := uint64(float64(currentRelayingFee) * float64(vsize) * overpayingRate)

	if newFee < oldFee || newFee > uint64(float64(oldFee)*DEFAULT_RATE_INCREASING_STEP) {
		newFee = uint64(float64(oldFee) * DEFAULT_RATE_INCREASING_STEP)
	}
	newFeePerUnshieldRequest := uint((newFee-1)/uint64(numberOfUnshieldRequest)+1) * 10 // newFeePerUnshieldRequest: nano pBTC

	return newFeePerUnshieldRequest
}

func IsEnoughFee(vsize int, feePerRequest uint, numberOfRequest uint, currentRelayingFee uint) bool {
	overpayingFeePercentStr := os.Getenv("OVERPAYING_FEE_PERCENT")
	overpayingFeePercent, _ := strconv.Atoi(overpayingFeePercentStr)
	overpayingRate := 1 + float64(overpayingFeePercent)/100

	fee := uint64(feePerRequest) * uint64(numberOfRequest) / 10 // fee: satoshi, feePerRequest: nano pToken
	expectedFee := uint64(float64(currentRelayingFee) * float64(vsize) * overpayingRate)
	return fee >= expectedFee
}
