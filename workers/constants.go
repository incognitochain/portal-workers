package workers

import "time"

const (
	// DefaultFee - default fee
	DefaultFee = 20

	BTCID                    = "4584d5e9b2fc0337dfb17f4b5bb025e5b82c38cfa4f54e8a3d4fcdd03954ff82"
	BTCConfirmationThreshold = 1

	NumGetStatusTries = 3
	IntervalTries     = 1 * time.Minute

	PortalReplacementFeeRequestMeta = 255
	PortalSubmitConfirmedTxMeta     = 256
)
