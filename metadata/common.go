package metadata

import (
	"encoding/json"

	"github.com/0xkraken/incognito-sdk-golang/metadata"
)

func calculateSize(meta metadata.Metadata) uint64 {
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return 0
	}
	return uint64(len(metaBytes))
}
