package entities

type FeeReplacementStatus struct {
	Status int
}

type FeeReplacementStatusRes struct {
	RPCBaseRes
	Result *FeeReplacementStatus
}
