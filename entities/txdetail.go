package entities

type TxDetail struct {
	IsInBlock bool
}

type TxDetailRes struct {
	RPCBaseRes
	Result *TxDetail
}
