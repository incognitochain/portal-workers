package entities

type ConfirmedTxStatus struct {
	Status int
}

type ConfirmedTxStatusRes struct {
	RPCBaseRes
	Result *ConfirmedTxStatus
}
