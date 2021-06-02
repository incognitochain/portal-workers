package entities

type ViewState struct {
	Height uint64
}

type AllViewRes struct {
	RPCBaseRes
	Result []*ViewState
}
