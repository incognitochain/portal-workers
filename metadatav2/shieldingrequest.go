package metadata

import (
	"github.com/inc-backend/go-incognito/common"
	"github.com/inc-backend/go-incognito/metadata"
)

// PortalShieldingRequest - portal user requests ptoken (after sending pubToken to multisig wallet)
// metadata - portal user sends shielding request - create normal tx with this metadata
type PortalShieldingRequest struct {
	metadata.MetadataBase
	TokenID         string // pTokenID in incognito chain
	IncogAddressStr string
	ShieldingProof  string
}

func NewPortalShieldingRequest(
	metaType int,
	tokenID string,
	incogAddressStr string,
	shieldingProof string) (*PortalShieldingRequest, error) {
	metadataBase := metadata.MetadataBase{
		Type: metaType,
	}
	shieldingRequestMeta := &PortalShieldingRequest{
		TokenID:         tokenID,
		IncogAddressStr: incogAddressStr,
		ShieldingProof:  shieldingProof,
	}
	shieldingRequestMeta.MetadataBase = metadataBase
	return shieldingRequestMeta, nil
}

func (shieldingReq PortalShieldingRequest) Hash() *common.Hash {
	record := shieldingReq.MetadataBase.Hash().String()
	record += shieldingReq.TokenID
	record += shieldingReq.IncogAddressStr
	record += shieldingReq.ShieldingProof
	// final hash
	hash := common.HashH([]byte(record))
	return &hash
}

func (shieldingReq *PortalShieldingRequest) CalculateSize() uint64 {
	return calculateSize(shieldingReq)
}

func (shieldingReq *PortalShieldingRequest) GetType() int {
	return shieldingReq.Type
}
