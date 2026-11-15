package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/channel"
	"github.com/vod2live/cyonlive_vod2live/internal/config"
	"github.com/vod2live/cyonlive_vod2live/internal/db"
	"github.com/vod2live/cyonlive_vod2live/internal/health"
	"github.com/vod2live/cyonlive_vod2live/internal/metrics"
	"github.com/vod2live/cyonlive_vod2live/internal/playout"
	"github.com/vod2live/cyonlive_vod2live/internal/retry"
	"github.com/vod2live/cyonlive_vod2live/internal/schedule"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := retry.Run(ctx, 8, 500*time.Millisecond, func() error { return redisClient.Ping(ctx).Err() }); err != nil {
		log.Fatal(err)
	}
	channels := channel.PostgresRepository{Pool: pool}
	playoutMetrics := &metrics.Metrics{}
	metricsAddr := os.Getenv("PLAYOUT_METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9092"
	}
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/metrics", playoutMetrics.Handler)
	metricsMux.HandleFunc("/healthz", health.Handler)
	metricsMux.HandleFunc("/readyz", health.ReadyHandler(pool, redisClient))
	metricsServer := &http.Server{Addr: metricsAddr, Handler: metricsMux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	metricsErr := make(chan error, 1)
	go func() { metricsErr <- metricsServer.ListenAndServe() }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()
	worker := playout.Worker{Store: playout.RedisStore{Client: redisClient}, Schedules: schedule.PostgresRepository{Pool: pool}, Assets: asset.PostgresRepository{Pool: pool}, Owner: os.Getenv("HOSTNAME"), Interval: 5 * time.Second, OnTick: func(err error) {
		playoutMetrics.ObservePlayoutTick(err)
		if err != nil {
			log.Printf("playout tick failed: %v", err)
		}
	}}
	log.Printf("vod2live playout worker started; metrics=%s", metricsAddr)
	if err := worker.Run(ctx, channels.ListRunning); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
	select {
	case err := <-metricsErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("playout metrics server: %v", err)
		}
	default:
	}
}
