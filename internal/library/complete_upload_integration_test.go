package library

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompleteUploadOversizePersistsFailureAndReleasesReservation(t *testing.T) {
	databaseURL := os.Getenv("VOD2LIVE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VOD2LIVE_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	orgID := uuid.New()
	libraryID := uuid.New()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `INSERT INTO organizations(id,name,slug) VALUES($1,$2,$3)`, orgID, "Complete upload test", "complete-upload-"+orgID.String()); err != nil {
		t.Fatalf("insert organization fixture: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO libraries(id,organization_id,name) VALUES($1,$2,$3)`, libraryID, orgID, "complete-upload"); err != nil {
		t.Fatalf("insert library fixture: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO storage_pools(organization_id,quota_bytes) VALUES($1,$2)`, orgID, 1024); err != nil {
		t.Fatalf("insert storage pool fixture: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatalf("commit fixture transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, cleanupErr := pool.Exec(cleanupCtx, `DELETE FROM organizations WHERE id=$1`, orgID)
		if cleanupErr != nil {
			t.Errorf("remove integration fixture: %v", cleanupErr)
		}
	})

	repo := Repository{Pool: pool}
	upload, err := repo.ReserveUpload(ctx, orgID, nil, libraryID, "oversize.mp4", "video/mp4", 100, "integration-test")
	if err != nil {
		t.Fatalf("reserve upload: %v", err)
	}
	if _, err = repo.CompleteUpload(ctx, orgID, upload.ID, 101); err == nil {
		t.Fatal("expected oversized upload to be rejected")
	}

	var status string
	var size, reserved, used int64
	if err = pool.QueryRow(ctx, `SELECT status,size_bytes FROM storage_objects WHERE id=$1`, upload.ID).Scan(&status, &size); err != nil {
		t.Fatalf("read object state: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT reserved_bytes,used_bytes FROM storage_pools WHERE organization_id=$1 AND tenant_id IS NULL`, orgID).Scan(&reserved, &used); err != nil {
		t.Fatalf("read quota state: %v", err)
	}
	if status != "failed" || size != 101 {
		t.Fatalf("object state status=%q size=%d, want failed/101", status, size)
	}
	if reserved != 0 || used != 0 {
		t.Fatalf("quota state reserved=%d used=%d, want 0/0", reserved, used)
	}
}
