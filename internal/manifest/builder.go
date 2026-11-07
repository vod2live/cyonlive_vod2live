package manifest

import (
	"github.com/google/uuid"
	"github.com/example/vod2live-backend/pkg/hls"
)

type Builder interface {
	BuildMaster(channelID uuid.UUID, variants []hls.Variant) ([]byte, error)
	BuildMedia(channelID, variantID uuid.UUID, window []hls.Segment, mediaSequence uint64) ([]byte, error)
}
