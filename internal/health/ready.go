package health

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func ReadyHandler(pool *pgxpool.Pool, cache *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{}
		status := http.StatusOK
		if err := pool.Ping(r.Context()); err != nil {
			checks["postgres"] = "down"
			status = http.StatusServiceUnavailable
		} else {
			checks["postgres"] = "up"
		}
		if err := cache.Ping(context.Background()).Err(); err != nil {
			checks["redis"] = "down"
			status = http.StatusServiceUnavailable
		} else {
			checks["redis"] = "up"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": map[bool]string{true: "ready", false: "not_ready"}[status == http.StatusOK], "checks": checks})
	}
}
