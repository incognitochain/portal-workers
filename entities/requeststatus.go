package entities

type RequestStatus struct {
	Status int
}

type RequestStatusRes struct {
	RPCBaseRes
	Result *RequestStatus
}
