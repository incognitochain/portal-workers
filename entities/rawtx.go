package entities

type SignedRawTx struct {
	SignedTx     string
	BeaconHeight uint64
}

type SignedRawTxRes struct {
	RPCBaseRes
	Result *SignedRawTx
}
