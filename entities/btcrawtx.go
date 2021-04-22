package entities

type SignedRawTx struct {
	SignedTx     string
	BeaconHeight uint64
	TxID         string
}

type SignedRawTxRes struct {
	RPCBaseRes
	Result *SignedRawTx
}
