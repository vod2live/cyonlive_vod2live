package schedule

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID        uuid.UUID  `json:"id"`
	ChannelID uuid.UUID  `json:"channel_id"`
	AssetID   uuid.UUID  `json:"asset_id"`
	StartsAt  time.Time  `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at,omitempty"`
	Position  int        `json:"position"`
	Enabled   bool       `json:"enabled"`
}

type PostgresRepository struct{ Pool *pgxpool.Pool }

func (r PostgresRepository) Create(ctx context.Context, item Item) error {
	var conflict bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schedule_items WHERE channel_id=$1 AND enabled=true AND tstzrange(starts_at,COALESCE(ends_at,'infinity'::timestamptz),'[)') && tstzrange($2,COALESCE($3,'infinity'::timestamptz),'[)'))`, item.ChannelID, item.StartsAt, item.EndsAt).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("schedule conflict")
	}
	_, err = r.Pool.Exec(ctx, `INSERT INTO schedule_items (id,channel_id,asset_id,starts_at,ends_at,position,enabled) VALUES ($1,$2,$3,$4,$5,$6,$7)`, item.ID, item.ChannelID, item.AssetID, item.StartsAt, item.EndsAt, item.Position, item.Enabled)
	return err
}

func (r PostgresRepository) List(ctx context.Context, channelID uuid.UUID) ([]Item, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,channel_id,asset_id,starts_at,ends_at,position,enabled FROM schedule_items WHERE channel_id=$1 ORDER BY starts_at,position`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.ChannelID, &item.AssetID, &item.StartsAt, &item.EndsAt, &item.Position, &item.Enabled); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
