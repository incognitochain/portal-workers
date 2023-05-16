package workers

import (
	"time"

	"github.com/incognitochain/go-incognito-sdk-v2/incclient"
)

const (
	BTCID                    = "b832e5d3b1f01a4f0623f7fe91d6673461e1f5d37d91fe78c5c2e6183ff39696"
	BTCConfirmationThreshold = 6

	NumGetStatusTries = 3
	IntervalTries     = 1 * time.Minute

	PortalReplacementFeeRequestMeta = 255
	PortalSubmitConfirmedTxMeta     = 256

	// status of unshield processing - used to store db
	PortalUnshieldReqWaitingStatus   = 0
	PortalUnshieldReqProcessedStatus = 1
	PortalUnshieldReqCompletedStatus = 2
	PortalUnshieldReqRefundedStatus  = 3
)

var DefaultNetworkFee = incclient.DefaultPRVFee
