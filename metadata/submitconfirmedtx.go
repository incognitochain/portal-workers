package metadata

import (
	"github.com/0xkraken/incognito-sdk-golang/common"
)

type PortalSubmitConfirmedTxRequest struct {
	MetadataBase
	TokenID       string // pTokenID in incognito chain
	UnshieldProof string
	BatchID       string
}

func NewPortalSubmitConfirmedTxRequest(metaType int, unshieldProof, tokenID, batchID string) (*PortalSubmitConfirmedTxRequest, error) {
	metadataBase := MetadataBase{
		Type: metaType,
	}
	portalUnshieldReq := &PortalSubmitConfirmedTxRequest{
		TokenID:       tokenID,
		BatchID:       batchID,
		UnshieldProof: unshieldProof,
	}
	portalUnshieldReq.MetadataBase = metadataBase
	return portalUnshieldReq, nil
}

func (r PortalSubmitConfirmedTxRequest) Hash() *common.Hash {
	record := r.MetadataBase.Hash().String()
	record += r.TokenID
	record += r.BatchID
	record += r.UnshieldProof

	// final hash
	hash := common.HashH([]byte(record))
	return &hash
}

func (r *PortalSubmitConfirmedTxRequest) CalculateSize() uint64 {
	return calculateSize(r)
}

func (r *PortalSubmitConfirmedTxRequest) GetType() int {
	return r.Type
}
