package schedule

import (
	"time"
	"github.com/google/uuid"
)

type Item struct { ID, ChannelID, AssetID uuid.UUID; StartsAt, EndsAt time.Time; Position int }

type Repository interface {
	Current(channelID uuid.UUID, at time.Time) (Item, error)
	Upcoming(channelID uuid.UUID, from time.Time, limit int) ([]Item, error)
}
