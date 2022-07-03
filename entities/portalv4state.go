package entities

type ProcessedUnshieldRequestBatch struct {
	BatchID      string
	UnshieldsID  []string
	ExternalFees map[uint64]*ExternalFeeInfo
	Utxos        []*UTXO
}

type UTXO struct {
	WalletAddress string
	TxHash        string
	OutputIdx     uint32
	OutputAmount  uint64
	ChainCodeSeed string // It's incognito address of user want to shield to Incognito
}

type WaitingUnshieldRequest struct {
	UnshieldID    string
	RemoteAddress string
	Amount        uint64
	BeaconHeight  uint64
}

type PortalV4State struct {
	WaitingUnshieldRequests   map[string]map[string]*WaitingUnshieldRequest        // tokenID : hash(tokenID || unshieldID) : value
	ProcessedUnshieldRequests map[string]map[string]*ProcessedUnshieldRequestBatch // tokenID : hash(tokenID || batchID) : value
	UTXOs                     map[string]map[string]*UTXO  // tokenID : hash(tokenID || walletAddress || txHash || index) : value
}

type PortalV4StateByHeightRes struct {
	RPCBaseRes
	Result *PortalV4State
}
