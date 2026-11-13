package media

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vod2live/cyonlive_vod2live/internal/library"
	"github.com/vod2live/cyonlive_vod2live/internal/storage"
)

const missingFFmpegPath = "/no/such/vod2live-test-ffmpeg"

func TestWorkerFailurePersistsDeadJobAndObjectState(t *testing.T) {
	databaseURL := os.Getenv("VOD2LIVE_TEST_DATABASE_URL")
	endpoint := os.Getenv("VOD2LIVE_TEST_MINIO_ENDPOINT")
	if databaseURL == "" || endpoint == "" {
		t.Skip("set VOD2LIVE_TEST_DATABASE_URL and VOD2LIVE_TEST_MINIO_ENDPOINT to run worker integration test")
	}
	accessKey := envOrDefault("VOD2LIVE_TEST_MINIO_ACCESS_KEY", "minioadmin")
	secretKey := envOrDefault("VOD2LIVE_TEST_MINIO_SECRET_KEY", "minioadmin")
	bucket := envOrDefault("VOD2LIVE_TEST_MINIO_BUCKET", "vod2live")
	store, err := storage.New(endpoint, accessKey, secretKey, bucket, false)
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure MinIO bucket: %v", err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	t.Run("missing MinIO source", func(t *testing.T) {
		runWorkerFailureCase(t, ctx, pool, store, true, "download source")
	})
	t.Run("FFmpeg executable unavailable", func(t *testing.T) {
		runWorkerFailureCase(t, ctx, pool, store, false, "ffmpeg")
	})
}

func runWorkerFailureCase(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *storage.Client, missingSource bool, wantError string) {
	t.Helper()
	orgID, libraryID, assetID, objectID, jobID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	objectKey := "integration-failures/" + objectID.String()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `INSERT INTO organizations(id,name,slug) VALUES($1,$2,$3)`, orgID, "Worker failure test", "worker-failure-"+orgID.String()); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO libraries(id,organization_id,name) VALUES($1,$2,$3)`, libraryID, orgID, "worker-failure"); err != nil {
		t.Fatalf("insert library: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO assets(id,organization_id,library_id,title,hls_url) VALUES($1,$2,$3,$4,$5)`, assetID, orgID, libraryID, "worker failure", ""); err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO storage_objects(id,organization_id,library_id,asset_id,bucket,object_key,content_type,status,size_bytes)
		VALUES($1,$2,$3,$4,$5,$6,'video/mp4','uploaded',4)`, objectID, orgID, libraryID, assetID, store.Bucket, objectKey); err != nil {
		t.Fatalf("insert storage object: %v", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_jobs(id,organization_id,asset_id,source_object_id,status,max_attempts)
		VALUES($1,$2,$3,$4,'queued',1)`, jobID, orgID, assetID, objectID); err != nil {
		t.Fatalf("insert media job: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit fixture transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = store.Delete(cleanupCtx, objectKey)
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
			{`DELETE FROM organizations WHERE id=$1`, orgID},
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

	if !missingSource {
		input, err := os.CreateTemp(t.TempDir(), "source-*.mp4")
		if err != nil {
			t.Fatalf("create source fixture: %v", err)
		}
		if _, err := input.Write([]byte("bad!")); err != nil {
			_ = input.Close()
			t.Fatalf("write source fixture: %v", err)
		}
		if err := input.Close(); err != nil {
			t.Fatalf("close source fixture: %v", err)
		}
		if err := store.UploadFile(ctx, objectKey, input.Name(), "video/mp4"); err != nil {
			t.Fatalf("upload source fixture: %v", err)
		}
	}

	jobs := library.Repository{Pool: pool}
	job, err := jobs.ClaimJob(ctx)
	if err != nil {
		t.Fatalf("claim fixture job: %v", err)
	}
	worker := Worker{Jobs: jobs, Storage: store, FFmpegPath: missingFFmpegPath, TempDir: t.TempDir()}
	processErr := worker.process(ctx, job)
	if processErr == nil || !strings.Contains(strings.ToLower(processErr.Error()), strings.ToLower(wantError)) {
		t.Fatalf("process error = %v, want substring %q", processErr, wantError)
	}
	if err := jobs.FinishJob(ctx, job, "", 0, processErr.Error(), false); err != nil {
		t.Fatalf("persist worker failure: %v", err)
	}

	var status, lastError, objectStatus string
	if err := pool.QueryRow(ctx, `SELECT j.status,j.last_error,o.status FROM media_jobs j JOIN storage_objects o ON o.id=j.source_object_id WHERE j.id=$1`, jobID).Scan(&status, &lastError, &objectStatus); err != nil {
		t.Fatalf("read persisted failure: %v", err)
	}
	if status != "dead" || objectStatus != "failed" {
		t.Fatalf("persisted states job=%q object=%q, want dead/failed", status, objectStatus)
	}
	if !strings.Contains(strings.ToLower(lastError), strings.ToLower(wantError)) {
		t.Fatalf("last_error = %q, want substring %q", lastError, wantError)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
