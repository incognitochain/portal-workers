package workers

import "time"

const (
	// DefaultFee - default fee
	DefaultFee = 20

	BTCID                 = "ef5947f70ead81a76a53c7c8b7317dd5245510c665d3a13921dc9a581188728b"
	ConfirmationThreshold = 6

	NUM_GET_STATUS_TRIES = 5
	INTERVAL_TRIES       = 1 * time.Minute

	PortalShieldingRequestMeta      = 250
	PortalReplacementFeeRequestMeta = 253
	PortalSubmitConfirmedTxMeta     = 254

	MultisigAddress = "2NGFTTKNj59NGmjQpajsEXGxwf9SP8gvJiv"
)
