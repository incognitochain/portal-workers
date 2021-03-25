package metadata

import (
	"strconv"

	"github.com/0xkraken/incognito-sdk-golang/common"
)

type PortalReplacementFeeRequest struct {
	MetadataBase
	TokenID string
	BatchID string
	Fee     uint
}

func NewPortalReplacementFeeRequest(metaType int, tokenID, batchID string, fee uint) (*PortalReplacementFeeRequest, error) {
	metadataBase := MetadataBase{
		Type: metaType,
	}
	portalUnshieldReq := &PortalReplacementFeeRequest{
		TokenID: tokenID,
		BatchID: batchID,
		Fee:     fee,
	}
	portalUnshieldReq.MetadataBase = metadataBase
	return portalUnshieldReq, nil
}

func (repl PortalReplacementFeeRequest) Hash() *common.Hash {
	record := repl.MetadataBase.Hash().String()
	record += repl.TokenID
	record += repl.BatchID
	record += strconv.FormatUint(uint64(repl.Fee), 10)

	// final hash
	hash := common.HashH([]byte(record))
	return &hash
}

func (repl *PortalReplacementFeeRequest) CalculateSize() uint64 {
	return calculateSize(repl)
}

func (repl *PortalReplacementFeeRequest) GetType() int {
	return repl.Type
}
