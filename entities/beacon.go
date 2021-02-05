package entities

type BeaconBestState struct {
	BeaconHeight uint64
}

type BeaconBestStateRes struct {
	RPCBaseRes
	Result *BeaconBestState
}

type BeaconBlockState struct {
	Instructions [][]string
}

type BeaconBlockByHeightRes struct {
	RPCBaseRes
	Result []*BeaconBlockState
}
