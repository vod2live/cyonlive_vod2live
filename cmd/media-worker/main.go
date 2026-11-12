package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vod2live/cyonlive_vod2live/internal/config"
	"github.com/vod2live/cyonlive_vod2live/internal/db"
	"github.com/vod2live/cyonlive_vod2live/internal/library"
	"github.com/vod2live/cyonlive_vod2live/internal/media"
	"github.com/vod2live/cyonlive_vod2live/internal/retry"
	"github.com/vod2live/cyonlive_vod2live/internal/storage"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	mediaCfg := config.LoadMedia()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	store, err := storage.New(mediaCfg.StorageEndpoint, mediaCfg.StorageAccessKey, mediaCfg.StorageSecretKey, mediaCfg.StorageBucket, mediaCfg.StorageUseSSL)
	if err != nil {
		log.Fatal(err)
	}
	if err = retry.Run(ctx, 8, 500*time.Millisecond, func() error { return store.EnsureBucket(ctx) }); err != nil {
		log.Fatal(err)
	}
	log.Println("vod2live media worker started")
	err = (media.Worker{Jobs: library.Repository{Pool: pool}, Storage: store, FFmpegPath: mediaCfg.FFmpegPath, TempDir: mediaCfg.MediaTempDir, Poll: time.Duration(mediaCfg.MediaPollSeconds) * time.Second, KeepSource: mediaCfg.KeepOriginal}).Run(ctx)
	if err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}
