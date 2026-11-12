package library

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ Pool *pgxpool.Pool }

type Library struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	TenantID       *uuid.UUID `json:"tenant_id,omitempty"`
	Name           string     `json:"name"`
}

type Quota struct {
	QuotaBytes     int64 `json:"quota_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	ReservedBytes  int64 `json:"reserved_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
}

type Upload struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	TenantID       *uuid.UUID `json:"tenant_id,omitempty"`
	LibraryID      uuid.UUID  `json:"library_id"`
	AssetID        uuid.UUID  `json:"asset_id"`
	ObjectKey      string     `json:"object_key"`
	ExpectedBytes  int64      `json:"expected_bytes"`
	Status         string     `json:"status"`
}

func (r Repository) CreateLibrary(ctx context.Context, org uuid.UUID, tenant *uuid.UUID, name string) (Library, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Library{}, fmt.Errorf("library name is required")
	}
	var item Library
	err := r.Pool.QueryRow(ctx, `INSERT INTO libraries (organization_id,tenant_id,name) VALUES ($1,$2,$3) RETURNING id,organization_id,tenant_id,name`, org, tenant, name).Scan(&item.ID, &item.OrganizationID, &item.TenantID, &item.Name)
	return item, err
}

func (r Repository) ListLibraries(ctx context.Context, org uuid.UUID) ([]Library, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,organization_id,tenant_id,name FROM libraries WHERE organization_id=$1 ORDER BY name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Library
	for rows.Next() {
		var x Library
		if err := rows.Scan(&x.ID, &x.OrganizationID, &x.TenantID, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r Repository) EnsureQuota(ctx context.Context, org uuid.UUID, tenant *uuid.UUID, bytes int64) error {
	if bytes < 0 {
		return fmt.Errorf("quota must not be negative")
	}
	if _, err := r.Pool.Exec(ctx, `INSERT INTO storage_pools (organization_id,tenant_id,quota_bytes) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, org, tenant, bytes); err != nil {
		return err
	}
	_, err := r.Pool.Exec(ctx, `UPDATE storage_pools SET quota_bytes=$1,updated_at=now() WHERE organization_id=$2 AND tenant_id IS NOT DISTINCT FROM $3`, bytes, org, tenant)
	return err
}

func (r Repository) Quota(ctx context.Context, org uuid.UUID, tenant *uuid.UUID) (Quota, error) {
	var q Quota
	err := r.Pool.QueryRow(ctx, `SELECT quota_bytes,used_bytes,reserved_bytes FROM storage_pools WHERE organization_id=$1 AND tenant_id IS NOT DISTINCT FROM $2`, org, tenant).Scan(&q.QuotaBytes, &q.UsedBytes, &q.ReservedBytes)
	if err != nil {
		return q, err
	}
	q.AvailableBytes = q.QuotaBytes - q.UsedBytes - q.ReservedBytes
	return q, nil
}

func (r Repository) ReserveUpload(ctx context.Context, org uuid.UUID, tenant *uuid.UUID, libraryID uuid.UUID, filename string, contentType string, expected int64, bucket string) (Upload, error) {
	if expected <= 0 {
		return Upload{}, fmt.Errorf("size_bytes must be positive")
	}
	if expected > 1<<40 {
		return Upload{}, fmt.Errorf("file is too large")
	}
	name := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if name == "." || name == "" {
		return Upload{}, fmt.Errorf("filename is required")
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Upload{}, err
	}
	defer tx.Rollback(ctx)
	var available int64
	if err = tx.QueryRow(ctx, `SELECT quota_bytes-used_bytes-reserved_bytes FROM storage_pools WHERE organization_id=$1 AND tenant_id IS NOT DISTINCT FROM $2 FOR UPDATE`, org, tenant).Scan(&available); err != nil {
		return Upload{}, fmt.Errorf("quota unavailable: %w", err)
	}
	if available < expected {
		return Upload{}, fmt.Errorf("storage quota exceeded")
	}
	var libraryTenant *uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT tenant_id FROM libraries WHERE id=$1 AND organization_id=$2`, libraryID, org).Scan(&libraryTenant); err != nil {
		return Upload{}, fmt.Errorf("library not found: %w", err)
	}
	if (libraryTenant == nil) != (tenant == nil) || libraryTenant != nil && tenant != nil && *libraryTenant != *tenant {
		return Upload{}, fmt.Errorf("library does not belong to tenant scope")
	}
	assetID, objectID := uuid.New(), uuid.New()
	key := fmt.Sprintf("quarantine/organization/%s/upload/%s/%s", org, objectID, name)
	if _, err = tx.Exec(ctx, `UPDATE storage_pools SET reserved_bytes=reserved_bytes+$1,updated_at=now() WHERE organization_id=$2 AND tenant_id IS NOT DISTINCT FROM $3`, expected, org, tenant); err != nil {
		return Upload{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO assets (id,organization_id,library_id,tenant_id,title,hls_url,status,approval_status,metadata) VALUES ($1,$2,$3,$4,$5,$6,'pending','pending',$7)`, assetID, org, libraryID, tenant, name, "", `{"source":"managed-upload"}`); err != nil {
		return Upload{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO storage_objects (id,organization_id,tenant_id,library_id,asset_id,bucket,object_key,content_type,expected_size_bytes,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'uploading')`, objectID, org, tenant, libraryID, assetID, bucket, key, contentType, expected); err != nil {
		return Upload{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Upload{}, err
	}
	return Upload{ID: objectID, OrganizationID: org, TenantID: tenant, LibraryID: libraryID, AssetID: assetID, ObjectKey: key, ExpectedBytes: expected, Status: "uploading"}, nil
}

func (r Repository) CompleteUpload(ctx context.Context, org, objectID uuid.UUID, actualBytes int64) (Upload, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Upload{}, err
	}
	defer tx.Rollback(ctx)
	var u Upload
	var old int64
	var tenant *uuid.UUID
	var libraryID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id,organization_id,tenant_id,library_id,asset_id,object_key,expected_size_bytes,status FROM storage_objects WHERE id=$1 AND organization_id=$2 FOR UPDATE`, objectID, org).Scan(&u.ID, &u.OrganizationID, &tenant, &libraryID, &u.AssetID, &u.ObjectKey, &u.ExpectedBytes, &u.Status)
	if err != nil {
		return Upload{}, err
	}
	u.TenantID = tenant
	u.LibraryID = libraryID
	if u.Status != "uploading" {
		return u, fmt.Errorf("upload is not pending")
	}
	if actualBytes <= 0 {
		return u, fmt.Errorf("uploaded object is empty")
	}
	if _, err = tx.Exec(ctx, `UPDATE storage_objects SET size_bytes=$1,status='uploaded',updated_at=now() WHERE id=$2`, actualBytes, objectID); err != nil {
		return u, err
	}
	if err = tx.QueryRow(ctx, `SELECT expected_size_bytes FROM storage_objects WHERE id=$1`, objectID).Scan(&old); err != nil {
		return u, err
	}
	if _, err = tx.Exec(ctx, `UPDATE storage_pools SET reserved_bytes=GREATEST(reserved_bytes-$1,0),used_bytes=used_bytes+$2,updated_at=now() WHERE organization_id=$3 AND tenant_id IS NOT DISTINCT FROM $4`, old, actualBytes, org, tenant); err != nil {
		return u, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_jobs (organization_id,tenant_id,asset_id,source_object_id) VALUES ($1,$2,$3,$4)`, org, tenant, u.AssetID, objectID); err != nil {
		return u, err
	}
	if err = tx.Commit(ctx); err != nil {
		return u, err
	}
	u.Status = "uploaded"
	return u, nil
}

func (r Repository) UploadKey(ctx context.Context, org, objectID uuid.UUID) (string, error) {
	var key string
	err := r.Pool.QueryRow(ctx, `SELECT object_key FROM storage_objects WHERE organization_id=$1 AND id=$2 AND status='uploading'`, org, objectID).Scan(&key)
	return key, err
}

type Job struct {
	ID, OrganizationID, AssetID, SourceObjectID uuid.UUID
	TenantID                                    *uuid.UUID
	ObjectKey, Bucket                           string
	SourceBytes                                 int64
}

func (r Repository) ClaimJob(ctx context.Context) (Job, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	var j Job
	var tenantID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT j.id,j.organization_id,j.asset_id,j.source_object_id,COALESCE(j.tenant_id,'00000000-0000-0000-0000-000000000000'),o.object_key,o.bucket,o.size_bytes FROM media_jobs j JOIN storage_objects o ON o.id=j.source_object_id WHERE j.status='queued' ORDER BY j.created_at FOR UPDATE OF j SKIP LOCKED LIMIT 1`).Scan(&j.ID, &j.OrganizationID, &j.AssetID, &j.SourceObjectID, &tenantID, &j.ObjectKey, &j.Bucket, &j.SourceBytes)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Job{}, err
		}
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE media_jobs SET status='processing',attempts=attempts+1,started_at=now(),locked_at=now(),updated_at=now() WHERE id=$1`, j.ID); err != nil {
		return Job{}, err
	}
	if tenantID != uuid.Nil {
		j.TenantID = &tenantID
	}
	if _, err = tx.Exec(ctx, `UPDATE storage_objects SET status='processing',updated_at=now() WHERE id=$1`, j.SourceObjectID); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return j, nil
}
func (r Repository) FinishJob(ctx context.Context, j Job, hlsURI string, storageBytes int64, errMsg string) error {
	status := "completed"
	if errMsg != "" {
		status = "failed"
	}
	_, err := r.Pool.Exec(ctx, `UPDATE media_jobs SET status=$1,progress_percent=$2,error_message=$3,completed_at=now(),updated_at=now() WHERE id=$4`, status, func() int {
		if status == "completed" {
			return 100
		}
		return 0
	}(), errMsg, j.ID)
	if err != nil {
		return err
	}
	if status == "completed" {
		_, err = r.Pool.Exec(ctx, `UPDATE storage_objects SET status='ready',updated_at=now() WHERE id=$1`, j.SourceObjectID)
		_, err = r.Pool.Exec(ctx, `UPDATE assets SET hls_url=$1,status='valid',storage_bytes=storage_bytes+$2,updated_at=now() WHERE id=$3`, hlsURI, storageBytes, j.AssetID)
	} else {
		_, err = r.Pool.Exec(ctx, `UPDATE storage_objects SET status='failed',updated_at=now() WHERE id=$1`, j.SourceObjectID)
	}
	return err
}

func (r Repository) AdjustUsage(ctx context.Context, org uuid.UUID, tenant *uuid.UUID, delta int64) error {
	_, err := r.Pool.Exec(ctx, `UPDATE storage_pools SET used_bytes=GREATEST(used_bytes+$1,0),updated_at=now() WHERE organization_id=$2 AND tenant_id IS NOT DISTINCT FROM $3`, delta, org, tenant)
	return err
}
