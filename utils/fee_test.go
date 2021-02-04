package utils

import (
	"testing"
)

func TestGetNewFee(t *testing.T) {
	newFee, err := GetNewFee(25, 2000)
	t.Logf("New fee: %v, err: %v", newFee, err)
	newFee, err = GetNewFee(25, 3000)
	t.Logf("New fee: %v, err: %v", newFee, err)
}
