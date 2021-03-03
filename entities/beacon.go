package entities

type BeaconBestState struct {
	BeaconHeight uint64
}

type BeaconBestStateRes struct {
	RPCBaseRes
	Result *BeaconBestState
}
