package entities

type ProcessedUnshieldRequestBatch struct {
	BatchID string
}

type PortalV4State struct {
	ProcessedUnshieldRequests map[string]map[string]*ProcessedUnshieldRequestBatch // tokenID : hash(tokenID || batchID) : value
}

type PortalV4StateByHeightRes struct {
	RPCBaseRes
	Result *PortalV4State
}
