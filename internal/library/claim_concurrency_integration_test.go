package library

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestClaimJobConcurrentWorkersClaimJobOnce(t *testing.T) {
	databaseURL := os.Getenv("VOD2LIVE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VOD2LIVE_TEST_DATABASE_URL to run PostgreSQL concurrency integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	poolConfig.MaxConns = 32
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	organizationID := uuid.New()
	libraryID := uuid.New()
	assetID := uuid.New()
	objectID := uuid.New()
	jobID := uuid.New()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `INSERT INTO organizations(id,name,slug) VALUES($1,$2,$3)`, organizationID, "Claim concurrency test", "claim-concurrency-"+organizationID.String())
	if err != nil {
		t.Fatalf("insert organization fixture: %v", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO libraries(id,organization_id,name) VALUES($1,$2,$3)`, libraryID, organizationID, "claim-concurrency")
	if err != nil {
		t.Fatalf("insert library fixture: %v", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO assets(id,organization_id,library_id,title,hls_url) VALUES($1,$2,$3,$4,$5)`, assetID, organizationID, libraryID, "claim concurrency", "")
	if err != nil {
		t.Fatalf("insert asset fixture: %v", err)
	}
	objectKey := "claim-concurrency/" + objectID.String()
	_, err = tx.Exec(ctx, `INSERT INTO storage_objects(id,organization_id,library_id,asset_id,bucket,object_key,content_type,status)
		VALUES($1,$2,$3,$4,'integration-test',$5,'video/mp4','uploaded')`, objectID, organizationID, libraryID, assetID, objectKey)
	if err != nil {
		t.Fatalf("insert storage object fixture: %v", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_jobs(id,organization_id,asset_id,source_object_id,status) VALUES($1,$2,$3,$4,'queued')`, jobID, organizationID, assetID, objectID)
	if err != nil {
		t.Fatalf("insert media job fixture: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit fixture transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		cleanupTx, cleanupErr := pool.Begin(cleanupCtx)
		if cleanupErr != nil {
			t.Errorf("begin fixture cleanup: %v", cleanupErr)
			return
		}
		defer cleanupTx.Rollback(cleanupCtx)
		for _, fixture := range []struct {
			query string
			id    uuid.UUID
		}{
			{`DELETE FROM media_jobs WHERE id=$1`, jobID},
			{`DELETE FROM storage_objects WHERE id=$1`, objectID},
			{`DELETE FROM assets WHERE id=$1`, assetID},
			{`DELETE FROM libraries WHERE id=$1`, libraryID},
			{`DELETE FROM organizations WHERE id=$1`, organizationID},
		} {
			if _, cleanupErr = cleanupTx.Exec(cleanupCtx, fixture.query, fixture.id); cleanupErr != nil {
				t.Errorf("remove integration fixture: %v", cleanupErr)
				return
			}
		}
		if cleanupErr = cleanupTx.Commit(cleanupCtx); cleanupErr != nil {
			t.Errorf("commit fixture cleanup: %v", cleanupErr)
		}
	})

	const workers = 24
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wait.Done()
			<-start
			job, claimErr := (Repository{Pool: pool}).ClaimJob(ctx)
			if claimErr == nil {
				if job.ID != jobID {
					results <- errors.New("worker claimed unexpected job")
					return
				}
				results <- nil
				return
			}
			if errors.Is(claimErr, pgx.ErrNoRows) {
				results <- pgx.ErrNoRows
				return
			}
			results <- claimErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	claimed, empty := 0, 0
	for result := range results {
		if result == nil {
			claimed++
		} else if errors.Is(result, pgx.ErrNoRows) {
			empty++
		} else {
			t.Errorf("ClaimJob returned unexpected error: %v", result)
		}
	}
	if claimed != 1 || empty != workers-1 {
		t.Fatalf("workers claimed=%d empty=%d, want claimed=1 empty=%d", claimed, empty, workers-1)
	}
}
