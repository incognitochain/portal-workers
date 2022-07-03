package entities

type RequestStatus struct {
	Status int
}

type RequestStatusRes struct {
	RPCBaseRes
	Result *RequestStatus
}

type ShieldingRequestStatus struct {
	Status int
	Error  string
}

type ShieldingRequestStatusRes struct {
	RPCBaseRes
	Result *ShieldingRequestStatus
}

type UnShieldingRequestStatus struct {
	Status int
	UnshieldAmount uint64
}

type UnShieldingRequestStatusRes struct {
	RPCBaseRes
	Result *UnShieldingRequestStatus
}


