package entities

type CreateTransactionResult struct {
	Base58CheckData string
	TxID            string
	TxType          string
	ShardID         byte
}

type SendRawTxRes struct {
	RPCBaseRes
	Result *CreateTransactionResult
}
