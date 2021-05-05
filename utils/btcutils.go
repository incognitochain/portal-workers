package utils

import (
	"encoding/hex"
)

// ConvertExternalBTCAmountToIncAmount converts amount in bTC chain (decimal 8) to amount in inc chain (decimal 9)
func ConvertExternalBTCAmountToIncAmount(externalBTCAmount int64) int64 {
	return externalBTCAmount * 10 // externalBTCAmount / 1^8 * 1^9
}

// ConvertIncPBTCAmountToExternalBTCAmount converts amount in inc chain (decimal 9) to amount in bTC chain (decimal 8)
func ConvertIncPBTCAmountToExternalBTCAmount(incPBTCAmount int64) int64 {
	return incPBTCAmount / 10 // incPBTCAmount / 1^9 * 1^8
}

func HexToBytes(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return []byte{}, err
	}
	return b, nil
}
