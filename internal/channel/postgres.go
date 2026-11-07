package channel

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Channel struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Status      string    `json:"status"`
	LoopEnabled bool      `json:"loop_enabled"`
}

type PostgresRepository struct{ Pool *pgxpool.Pool }

func (r PostgresRepository) Create(ctx context.Context, item Channel) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO channels (id,name,slug,status,loop_enabled) VALUES ($1,$2,$3,$4,$5)`, item.ID, item.Name, item.Slug, item.Status, item.LoopEnabled)
	return err
}

func (r PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (Channel, error) {
	var item Channel
	err := r.Pool.QueryRow(ctx, `SELECT id,name,slug,status,loop_enabled FROM channels WHERE id=$1`, id).Scan(&item.ID, &item.Name, &item.Slug, &item.Status, &item.LoopEnabled)
	if err != nil {
		return Channel{}, fmt.Errorf("find channel: %w", err)
	}
	return item, nil
}

func (r PostgresRepository) List(ctx context.Context) ([]Channel, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,name,slug,status,loop_enabled FROM channels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Channel
	for rows.Next() {
		var item Channel
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.Status, &item.LoopEnabled); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r PostgresRepository) ListRunning(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id FROM channels WHERE status='running' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (r PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.Pool.Exec(ctx, `UPDATE channels SET status=$2,updated_at=now() WHERE id=$1`, id, status)
	return err
}

func NormalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, " ", "-")))
}
