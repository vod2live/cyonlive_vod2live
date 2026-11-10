package playout

import (
	"github.com/google/uuid"
	"time"
)

type State struct {
	ChannelID, ScheduleItemID, AssetID uuid.UUID
	Status                             string
	PositionMS, MediaSequence          int64
	FencingToken                       uint64
	UpdatedAt                          time.Time
}

type StateStore interface {
	Get(channelID uuid.UUID) (State, error)
	Put(state State) error
	AcquireLock(channelID uuid.UUID, ttl time.Duration) (bool, error)
}
