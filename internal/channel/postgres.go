package channel

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Channel struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	TenantID       *uuid.UUID `json:"tenant_id,omitempty"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Status         string     `json:"status"`
	LoopEnabled    bool       `json:"loop_enabled"`
}

type PostgresRepository struct{ Pool *pgxpool.Pool }

func (r PostgresRepository) Create(ctx context.Context, item Channel) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO channels (id,organization_id,tenant_id,name,slug,status,loop_enabled) VALUES ($1,$2,$3,$4,$5,$6,$7)`, item.ID, item.OrganizationID, item.TenantID, item.Name, item.Slug, item.Status, item.LoopEnabled)
	return err
}

// FindByID is reserved for trusted internal services. HTTP handlers must use FindByIDInOrg.
func (r PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (Channel, error) {
	return r.find(ctx, `SELECT id,organization_id,tenant_id,name,slug,status,loop_enabled FROM channels WHERE id=$1`, id)
}

func (r PostgresRepository) FindByIDInOrg(ctx context.Context, organizationID, id uuid.UUID) (Channel, error) {
	return r.find(ctx, `SELECT id,organization_id,tenant_id,name,slug,status,loop_enabled FROM channels WHERE organization_id=$1 AND id=$2`, organizationID, id)
}

func (r PostgresRepository) find(ctx context.Context, query string, args ...any) (Channel, error) {
	var item Channel
	err := r.Pool.QueryRow(ctx, query, args...).Scan(&item.ID, &item.OrganizationID, &item.TenantID, &item.Name, &item.Slug, &item.Status, &item.LoopEnabled)
	if err != nil {
		return Channel{}, fmt.Errorf("find channel: %w", err)
	}
	return item, nil
}

func (r PostgresRepository) List(ctx context.Context) ([]Channel, error) {
	return r.list(ctx, `SELECT id,organization_id,tenant_id,name,slug,status,loop_enabled FROM channels ORDER BY name`)
}

func (r PostgresRepository) ListInOrg(ctx context.Context, organizationID uuid.UUID) ([]Channel, error) {
	return r.list(ctx, `SELECT id,organization_id,tenant_id,name,slug,status,loop_enabled FROM channels WHERE organization_id=$1 ORDER BY name`, organizationID)
}

// ListVisibleToUser returns organization-wide channels and channels belonging
// to a tenant in which the user has an explicit membership. The scope is
// enforced in SQL so HTTP list endpoints do not perform one authorization
// query per returned channel.
func (r PostgresRepository) ListVisibleToUser(ctx context.Context, userID, organizationID uuid.UUID) ([]Channel, error) {
	return r.list(ctx, `SELECT c.id,c.organization_id,c.tenant_id,c.name,c.slug,c.status,c.loop_enabled
		FROM channels c
		WHERE c.organization_id=$1
		AND (c.tenant_id IS NULL OR EXISTS (
			SELECT 1 FROM tenant_memberships tm
			WHERE tm.tenant_id=c.tenant_id AND tm.organization_id=c.organization_id AND tm.user_id=$2
		))
		ORDER BY c.name`, organizationID, userID)
}

func (r PostgresRepository) list(ctx context.Context, query string, args ...any) ([]Channel, error) {
	rows, err := r.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Channel
	for rows.Next() {
		var item Channel
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.TenantID, &item.Name, &item.Slug, &item.Status, &item.LoopEnabled); err != nil {
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

func (r PostgresRepository) UpdateStatusInOrg(ctx context.Context, organizationID, id uuid.UUID, status string) error {
	_, err := r.Pool.Exec(ctx, `UPDATE channels SET status=$3,updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, id, status)
	return err
}

func NormalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, " ", "-")))
}
