package entities

type TxDetail struct {
	IsInBlock   bool
	BlockHeight uint64
}

type TxDetailRes struct {
	RPCBaseRes
	Result *TxDetail
}
