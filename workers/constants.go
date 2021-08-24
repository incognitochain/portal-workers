package workers

import "time"

const (
	BTCID                    = "b832e5d3b1f01a4f0623f7fe91d6673461e1f5d37d91fe78c5c2e6183ff39696"
	BTCConfirmationThreshold = 6

	NumGetStatusTries = 3
	IntervalTries     = 1 * time.Minute
)
