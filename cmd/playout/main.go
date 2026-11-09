package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/channel"
	"github.com/vod2live/cyonlive_vod2live/internal/config"
	"github.com/vod2live/cyonlive_vod2live/internal/db"
	"github.com/vod2live/cyonlive_vod2live/internal/playout"
	"github.com/vod2live/cyonlive_vod2live/internal/schedule"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal(err)
	}
	channels := channel.PostgresRepository{Pool: pool}
	worker := playout.Worker{Store: playout.RedisStore{Client: redisClient}, Schedules: schedule.PostgresRepository{Pool: pool}, Assets: asset.PostgresRepository{Pool: pool}, Owner: os.Getenv("HOSTNAME"), Interval: 5 * time.Second}
	log.Println("vod2live playout worker started")
	if err := worker.Run(ctx, channels.ListRunning); err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}
