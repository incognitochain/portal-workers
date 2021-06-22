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

type ConvertingVauleRequestStatus struct {
	Status   int
	ErrorMsg string
}

type ConvertingVauleRequestStatusRes struct {
	RPCBaseRes
	Result *ConvertingVauleRequestStatus
}
