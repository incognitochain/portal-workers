package entities

type ExternalFeeInfo struct {
	NetworkFee    uint
	RBFReqIncTxID string
}

type UnshieldingBatchStatus struct {
	Status      int
	UnshieldIDs []string
	NetworkFees map[uint64]*ExternalFeeInfo
}

type UnshieldingBatchStatusRes struct {
	RPCBaseRes
	Result *UnshieldingBatchStatus
}
