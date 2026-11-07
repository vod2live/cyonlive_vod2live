package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/config"
	"github.com/vod2live/cyonlive_vod2live/internal/db"
	"github.com/vod2live/cyonlive_vod2live/internal/health"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil { log.Fatalf("database: %v", err) }
	defer pool.Close()
	repo := asset.PostgresRepository{Pool: pool}
	r := chi.NewRouter()
	r.Get("/healthz", health.Handler)
	r.Post("/v1/assets", createAsset(repo))
	r.Get("/v1/assets/{id}", getAsset(repo))
	r.Post("/v1/assets/{id}/validate", validateAsset(repo))
	log.Printf("vod2live api listening on %s", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, r))
}

type assetRequest struct { Title string `json:"title"`; HLSURL string `json:"hls_url"` }

func createAsset(repo asset.PostgresRepository) http.HandlerFunc { return func(w http.ResponseWriter, r *http.Request) {
	var req assetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.HLSURL) == "" { http.Error(w, "title e hls_url são obrigatórios", http.StatusBadRequest); return }
	item := asset.Asset{ID: uuid.New(), Title: req.Title, HLSURL: strings.TrimSpace(req.HLSURL), Status: asset.StatusPending}
	if err := repo.Create(r.Context(), item); err != nil { http.Error(w, "erro ao criar asset", 500); return }
	writeJSON(w, http.StatusCreated, item)
} }

func getAsset(repo asset.PostgresRepository) http.HandlerFunc { return func(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id")); if err != nil { http.Error(w, "id inválido", 400); return }
	item, err := repo.FindByID(r.Context(), id); if err != nil { http.Error(w, "asset não encontrado", 404); return }
	writeJSON(w, 200, item)
} }

func validateAsset(repo asset.PostgresRepository) http.HandlerFunc { return func(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id")); if err != nil { http.Error(w, "id inválido", 400); return }
	item, err := repo.FindByID(r.Context(), id); if err != nil { http.Error(w, "asset não encontrado", 404); return }
	inspection, err := (asset.HLSInspector{}).Inspect(item.HLSURL)
	if err != nil { _ = repo.UpdateStatus(r.Context(), id, asset.StatusInvalid, 0, map[string]string{"error": err.Error()}); http.Error(w, err.Error(), 422); return }
	_ = repo.UpdateStatus(r.Context(), id, asset.StatusValid, inspection.DurationMS, inspection)
	writeJSON(w, 200, inspection)
} }

func writeJSON(w http.ResponseWriter, status int, value any) { w.Header().Set("Content-Type", "application/json"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(value) }
