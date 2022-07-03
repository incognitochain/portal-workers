package entities

type BridgeTokenInfo struct {
	TokenID         string       `json:"tokenId"`
	Amount          uint64       `json:"amount"`
	ExternalTokenID []byte       `json:"externalTokenId"`
	Network         string       `json:"network"`
	IsCentralized   bool         `json:"isCentralized"`
}

type BridgeTokenInfoRes struct {
	RPCBaseRes
	Result []*BridgeTokenInfo
}