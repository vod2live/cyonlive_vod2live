package asset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrReviewNotAllowed = errors.New("asset is not ready for review")

type PostgresRepository struct{ Pool *pgxpool.Pool }

func (r PostgresRepository) Create(ctx context.Context, organizationID uuid.UUID, item Asset) error {
	metadata, err := json.Marshal(item.Metadata)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx, `INSERT INTO assets (id,organization_id,title,hls_url,status,metadata) VALUES ($1,$2,$3,$4,$5,$6)`, item.ID, organizationID, item.Title, item.HLSURL, item.Status, metadata)
	return err
}

// FindByID is reserved for trusted internal workers which already hold a schedule
// item; HTTP handlers must use FindByIDInOrg.
func (r PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (Asset, error) {
	return r.find(ctx, `SELECT id,organization_id,library_id,tenant_id,title,hls_url,status,approval_status,COALESCE(duration_ms,0),metadata FROM assets WHERE id=$1`, id)
}

func (r PostgresRepository) FindByIDInOrg(ctx context.Context, organizationID, id uuid.UUID) (Asset, error) {
	return r.find(ctx, `SELECT id,organization_id,library_id,tenant_id,title,hls_url,status,approval_status,COALESCE(duration_ms,0),metadata FROM assets WHERE organization_id=$1 AND id=$2`, organizationID, id)
}

func (r PostgresRepository) ListAwaitingReview(ctx context.Context, organizationID uuid.UUID) ([]Asset, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,organization_id,library_id,tenant_id,title,hls_url,status,approval_status,COALESCE(duration_ms,0),metadata
		FROM assets WHERE organization_id=$1 AND library_id IS NOT NULL AND status='valid' AND approval_status='pending'
		ORDER BY created_at LIMIT 200`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Asset
	for rows.Next() {
		var item Asset
		var raw []byte
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.LibraryID, &item.TenantID, &item.Title, &item.HLSURL, &item.Status, &item.ApprovalStatus, &item.DurationMS, &raw); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &item.Metadata)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r PostgresRepository) find(ctx context.Context, query string, args ...any) (Asset, error) {
	var item Asset
	var raw []byte
	err := r.Pool.QueryRow(ctx, query, args...).Scan(&item.ID, &item.OrganizationID, &item.LibraryID, &item.TenantID, &item.Title, &item.HLSURL, &item.Status, &item.ApprovalStatus, &item.DurationMS, &raw)
	if err != nil {
		return Asset{}, fmt.Errorf("find asset: %w", err)
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &item.Metadata)
	}
	return item, nil
}

func (r PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status, durationMS int64, metadata any) error {
	return r.updateStatus(ctx, `UPDATE assets SET status=$2,duration_ms=$3,metadata=$4,updated_at=now() WHERE id=$1`, id, status, durationMS, metadata)
}

func (r PostgresRepository) UpdateStatusInOrg(ctx context.Context, organizationID, id uuid.UUID, status Status, durationMS int64, metadata any) error {
	return r.updateStatus(ctx, `UPDATE assets SET status=$2,duration_ms=$3,metadata=$4,updated_at=now() WHERE id=$1 AND organization_id=$5`, id, status, durationMS, metadata, organizationID)
}

func (r PostgresRepository) updateStatus(ctx context.Context, query string, args ...any) error {
	metadata := args[3]
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	args[3] = raw
	_, err = r.Pool.Exec(ctx, query, args...)
	return err
}

func (r PostgresRepository) Review(ctx context.Context, organizationID, assetID, reviewerID uuid.UUID, approved bool, note string) error {
	status := ApprovalRejected
	if approved {
		status = ApprovalApproved
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE assets SET approval_status=$1,reviewed_by=$2,reviewed_at=now(),review_note=$3,updated_at=now() WHERE id=$4 AND organization_id=$5 AND library_id IS NOT NULL AND status='valid' AND approval_status='pending'`, status, reviewerID, strings.TrimSpace(note), assetID, organizationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrReviewNotAllowed
	}
	if !approved {
		if _, err = tx.Exec(ctx, `UPDATE schedule_items SET enabled=false WHERE organization_id=$1 AND asset_id=$2`, organizationID, assetID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
