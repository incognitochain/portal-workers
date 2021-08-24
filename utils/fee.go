package utils

const (
	OVERPAYING_RATE              = 1.1
	DEFAULT_RATE_INCREASING_STEP = 1.23
)

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
