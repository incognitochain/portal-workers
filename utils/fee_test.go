package utils

import (
	"testing"
)

func TestGetNewFee(t *testing.T) {
	newFee, err := GetNewFee(30, 25, 40)
	t.Logf("New fee: %v, err: %v", newFee, err)
	newFee, err = GetNewFee(30, 25, 40)
	t.Logf("New fee: %v, err: %v", newFee, err)
}
