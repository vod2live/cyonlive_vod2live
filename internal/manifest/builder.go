package manifest

import (
	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/pkg/hls"
)

type Builder interface {
	BuildMaster(channelID uuid.UUID, variants []hls.Variant) ([]byte, error)
	BuildMedia(channelID, variantID uuid.UUID, window []hls.Segment, mediaSequence uint64) ([]byte, error)
}
