package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/auth"
	"github.com/vod2live/cyonlive_vod2live/internal/channel"
	"github.com/vod2live/cyonlive_vod2live/internal/config"
	"github.com/vod2live/cyonlive_vod2live/internal/db"
	"github.com/vod2live/cyonlive_vod2live/internal/health"
	"github.com/vod2live/cyonlive_vod2live/internal/manifest"
	"github.com/vod2live/cyonlive_vod2live/internal/metrics"
	"github.com/vod2live/cyonlive_vod2live/internal/playout"
	"github.com/vod2live/cyonlive_vod2live/internal/schedule"
)

func main() {
	cfg := config.Load()
	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	assets := asset.PostgresRepository{Pool: pool}
	channels := channel.PostgresRepository{Pool: pool}
	schedules := schedule.PostgresRepository{Pool: pool}
	manifests := manifest.Service{Assets: assets, Schedules: schedules, Cache: manifest.ManifestCache{Client: redisClient}}
	playoutStore := playout.RedisStore{Client: redisClient}
	apiMetrics := &metrics.Metrics{}
	r := chi.NewRouter()
	r.Use(apiMetrics.Middleware)
	r.Get("/healthz", health.Handler)
	r.Get("/readyz", health.ReadyHandler(pool, redisClient))
	r.Get("/metrics", apiMetrics.Handler)
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(auth.APIKey(cfg.APIKey))
		v1.Post("/assets", createAsset(assets))
		v1.Get("/assets/{id}", getAsset(assets))
		v1.Post("/assets/{id}/validate", validateAsset(assets))
		v1.Post("/channels", createChannel(channels))
		v1.Get("/channels", listChannels(channels))
		v1.Get("/channels/{id}", getChannel(channels))
		v1.Post("/channels/{id}/start", updateChannelStatus(channels, "running"))
		v1.Post("/channels/{id}/stop", updateChannelStatus(channels, "stopped"))
		v1.Get("/channels/{id}/status", channelStatus(channels, playoutStore))
		v1.Get("/channels/{id}/master.m3u8", masterManifest(manifests))
		v1.Get("/channels/{id}/media/{variant}.m3u8", mediaManifest(manifests))
		v1.Post("/channels/{id}/schedule", createScheduleItem(channels, assets, schedules))
		v1.Get("/channels/{id}/schedule", listSchedule(schedules))
		v1.Patch("/schedule/{id}", updateScheduleItem(schedules))
		v1.Delete("/schedule/{id}", deleteScheduleItem(schedules))
	})
	log.Printf("vod2live api listening on %s", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, r))
}

type assetRequest struct {
	Title  string `json:"title"`
	HLSURL string `json:"hls_url"`
}

func createAsset(repo asset.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req assetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.HLSURL) == "" {
			http.Error(w, "title e hls_url são obrigatórios", 400)
			return
		}
		item := asset.Asset{ID: uuid.New(), Title: req.Title, HLSURL: strings.TrimSpace(req.HLSURL), Status: asset.StatusPending}
		if err := repo.Create(r.Context(), item); err != nil {
			http.Error(w, "erro ao criar asset", 500)
			return
		}
		writeJSON(w, 201, item)
	}
}
func getAsset(repo asset.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		item, err := repo.FindByID(r.Context(), id)
		if err != nil {
			http.Error(w, "asset não encontrado", 404)
			return
		}
		writeJSON(w, 200, item)
	}
}
func validateAsset(repo asset.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		item, err := repo.FindByID(r.Context(), id)
		if err != nil {
			http.Error(w, "asset não encontrado", 404)
			return
		}
		inspection, err := (asset.HLSInspector{}).Inspect(item.HLSURL)
		if err != nil {
			_ = repo.UpdateStatus(r.Context(), id, asset.StatusInvalid, 0, map[string]string{"error": err.Error()})
			http.Error(w, err.Error(), 422)
			return
		}
		_ = repo.UpdateStatus(r.Context(), id, asset.StatusValid, inspection.DurationMS, inspection)
		writeJSON(w, 200, inspection)
	}
}

type channelRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	LoopEnabled bool   `json:"loop_enabled"`
}

func createChannel(repo channel.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req channelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name é obrigatório", 400)
			return
		}
		slug := channel.NormalizeSlug(req.Slug)
		if slug == "" {
			slug = channel.NormalizeSlug(req.Name)
		}
		item := channel.Channel{ID: uuid.New(), Name: req.Name, Slug: slug, Status: "draft", LoopEnabled: req.LoopEnabled}
		if err := repo.Create(r.Context(), item); err != nil {
			http.Error(w, "erro ao criar canal", 500)
			return
		}
		writeJSON(w, 201, item)
	}
}
func listChannels(repo channel.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := repo.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao listar canais", 500)
			return
		}
		writeJSON(w, 200, items)
	}
}
func getChannel(repo channel.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		item, err := repo.FindByID(r.Context(), id)
		if err != nil {
			http.Error(w, "canal não encontrado", 404)
			return
		}
		writeJSON(w, 200, item)
	}
}

func updateChannelStatus(repo channel.PostgresRepository, status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		if _, err = repo.FindByID(r.Context(), id); err != nil {
			http.Error(w, "canal não encontrado", 404)
			return
		}
		if err = repo.UpdateStatus(r.Context(), id, status); err != nil {
			http.Error(w, "erro ao atualizar canal", 500)
			return
		}
		writeJSON(w, 200, map[string]string{"channel_id": id.String(), "status": status})
	}
}

func channelStatus(repo channel.PostgresRepository, store playout.RedisStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		item, err := repo.FindByID(r.Context(), id)
		if err != nil {
			http.Error(w, "canal não encontrado", 404)
			return
		}
		state, stateErr := store.Get(r.Context(), id)
		response := map[string]any{"channel": item, "playout": state}
		if stateErr != nil {
			response["playout_error"] = stateErr.Error()
		}
		writeJSON(w, 200, response)
	}
}

type scheduleRequest struct {
	AssetID  uuid.UUID  `json:"asset_id"`
	StartsAt time.Time  `json:"starts_at"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
	Position int        `json:"position"`
}

func createScheduleItem(channels channel.PostgresRepository, assets asset.PostgresRepository, repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id de canal inválido", 400)
			return
		}
		if _, err = channels.FindByID(r.Context(), channelID); err != nil {
			http.Error(w, "canal não encontrado", 404)
			return
		}
		var req scheduleRequest
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil || req.AssetID == uuid.Nil || req.StartsAt.IsZero() {
			http.Error(w, "asset_id e starts_at são obrigatórios", 400)
			return
		}
		assetItem, err := assets.FindByID(r.Context(), req.AssetID)
		if err != nil || assetItem.Status != asset.StatusValid {
			http.Error(w, "asset precisa estar validado", 422)
			return
		}
		if req.EndsAt == nil && assetItem.DurationMS > 0 {
			derived := req.StartsAt.Add(time.Duration(assetItem.DurationMS) * time.Millisecond)
			req.EndsAt = &derived
		}
		item := schedule.Item{ID: uuid.New(), ChannelID: channelID, AssetID: req.AssetID, StartsAt: req.StartsAt, EndsAt: req.EndsAt, Position: req.Position, Enabled: true}
		if err := repo.Create(r.Context(), item); err != nil {
			if errors.Is(err, context.Canceled) {
				http.Error(w, "requisição cancelada", 499)
			} else if strings.Contains(err.Error(), "schedule conflict") {
				http.Error(w, "conflito de horário na grade", 409)
			} else {
				http.Error(w, "erro ao criar item da grade", 500)
			}
			return
		}
		writeJSON(w, 201, item)
	}
}
func listSchedule(repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		items, err := repo.List(r.Context(), id)
		if err != nil {
			http.Error(w, "erro ao listar grade", 500)
			return
		}
		writeJSON(w, 200, items)
	}
}

type scheduleUpdateRequest struct {
	StartsAt time.Time  `json:"starts_at"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
	Position int        `json:"position"`
	Enabled  bool       `json:"enabled"`
}

func updateScheduleItem(repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		var req scheduleUpdateRequest
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil || req.StartsAt.IsZero() {
			http.Error(w, "starts_at é obrigatório", 400)
			return
		}
		if err = repo.Update(r.Context(), id, req.StartsAt, req.EndsAt, req.Position, req.Enabled); err != nil {
			if strings.Contains(err.Error(), "schedule conflict") {
				http.Error(w, "conflito de horário na grade", 409)
			} else {
				http.Error(w, "erro ao atualizar item da grade", 500)
			}
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "starts_at": req.StartsAt, "ends_at": req.EndsAt, "position": req.Position, "enabled": req.Enabled})
	}
}

func deleteScheduleItem(repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		if err = repo.Delete(r.Context(), id); err != nil {
			http.Error(w, "erro ao remover item da grade", 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func masterManifest(service manifest.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		body, err := service.Master(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write(body)
	}
}
func mediaManifest(service manifest.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		body, err := service.Media(r.Context(), id, chi.URLParam(r, "variant"), time.Now().UTC())
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write(body)
	}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
