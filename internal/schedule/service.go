package schedule

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, item Item) error
	List(ctx context.Context, channelID uuid.UUID) ([]Item, error)
	Current(ctx context.Context, channelID uuid.UUID, at time.Time) (Item, error)
}
