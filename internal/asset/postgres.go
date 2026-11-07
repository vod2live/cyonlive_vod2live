package asset

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct { Pool *pgxpool.Pool }

func (r PostgresRepository) Create(ctx context.Context, item Asset) error {
	metadata, err := json.Marshal(item.Metadata)
	if err != nil { return err }
	_, err = r.Pool.Exec(ctx, `INSERT INTO assets (id,title,hls_url,status,metadata) VALUES ($1,$2,$3,$4,$5)`, item.ID, item.Title, item.HLSURL, item.Status, metadata)
	return err
}

func (r PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (Asset, error) {
	var item Asset
	var raw []byte
	err := r.Pool.QueryRow(ctx, `SELECT id,title,hls_url,status,COALESCE(duration_ms,0),metadata FROM assets WHERE id=$1`, id).Scan(&item.ID, &item.Title, &item.HLSURL, &item.Status, &item.DurationMS, &raw)
	if err != nil { return Asset{}, fmt.Errorf("find asset: %w", err) }
	if len(raw) > 0 { _ = json.Unmarshal(raw, &item.Metadata) }
	return item, nil
}

func (r PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status, durationMS int64, metadata any) error {
	raw, err := json.Marshal(metadata); if err != nil { return err }
	_, err = r.Pool.Exec(ctx, `UPDATE assets SET status=$2,duration_ms=$3,metadata=$4,updated_at=now() WHERE id=$1`, id, status, durationMS, raw)
	return err
}
