package entities

type GetBlockChainInfoResult struct {
	ChainName    string                   `json:"ChainName"`
	BestBlocks   map[int]GetBestBlockItem `json:"BestBlocks"`
	// ActiveShards int                      `json:"ActiveShards"`
}

type GetBestBlockItem struct {
	Height              uint64 `json:"Height"`
	// Hash                string `json:"Hash"`
	// TotalTxs            uint64 `json:"TotalTxs"`
	// BlockProducer       string `json:"BlockProducer"`
	// ValidationData      string `json:"ValidationData"`
	// Epoch               uint64 `json:"Epoch"`
	// Time                int64  `json:"Time"`
	// RemainingBlockEpoch uint64 `json:"RemainingBlockEpoch"`
	// EpochBlock          uint64 `json:"EpochBlock"`
}

type GetBlockChainInfoRes struct {
	RPCBaseRes
	Result *GetBlockChainInfoResult
}

