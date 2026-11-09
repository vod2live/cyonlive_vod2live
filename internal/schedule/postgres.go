package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("schedule item not found")

type Item struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	ChannelID      uuid.UUID  `json:"channel_id"`
	AssetID        uuid.UUID  `json:"asset_id"`
	StartsAt       time.Time  `json:"starts_at"`
	EndsAt         *time.Time `json:"ends_at,omitempty"`
	Position       int        `json:"position"`
	Enabled        bool       `json:"enabled"`
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
	_, err = r.Pool.Exec(ctx, `INSERT INTO schedule_items (id,organization_id,channel_id,asset_id,starts_at,ends_at,position,enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, item.ID, item.OrganizationID, item.ChannelID, item.AssetID, item.StartsAt, item.EndsAt, item.Position, item.Enabled)
	return err
}

func (r PostgresRepository) List(ctx context.Context, channelID uuid.UUID) ([]Item, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,organization_id,channel_id,asset_id,starts_at,ends_at,position,enabled FROM schedule_items WHERE channel_id=$1 ORDER BY starts_at,position`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.ChannelID, &item.AssetID, &item.StartsAt, &item.EndsAt, &item.Position, &item.Enabled); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r PostgresRepository) Update(ctx context.Context, organizationID, id uuid.UUID, startsAt time.Time, endsAt *time.Time, position int, enabled bool) error {
	var conflict bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schedule_items WHERE organization_id=$1 AND channel_id=(SELECT channel_id FROM schedule_items WHERE id=$2 AND organization_id=$1) AND id<>$2 AND enabled=true AND tstzrange(starts_at,COALESCE(ends_at,'infinity'::timestamptz),'[)') && tstzrange($3,COALESCE($4,'infinity'::timestamptz),'[)'))`, organizationID, id, startsAt, endsAt).Scan(&conflict)
	if err != nil {
		return err
	}
	if conflict {
		return fmt.Errorf("schedule conflict")
	}
	result, err := r.Pool.Exec(ctx, `UPDATE schedule_items SET starts_at=$3,ends_at=$4,position=$5,enabled=$6 WHERE id=$1 AND organization_id=$2`, id, organizationID, startsAt, endsAt, position, enabled)
	if err == nil && result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r PostgresRepository) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	result, err := r.Pool.Exec(ctx, `DELETE FROM schedule_items WHERE id=$1 AND organization_id=$2`, id, organizationID)
	if err == nil && result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
