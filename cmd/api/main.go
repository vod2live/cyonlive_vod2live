package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/example/vod2live-backend/internal/asset"
	"github.com/example/vod2live-backend/internal/config"
	"github.com/example/vod2live-backend/internal/health"
)

func main() {
	cfg := config.Load()
	r := chi.NewRouter()
	r.Get("/healthz", health.Handler)
	r.Post("/v1/assets", createAsset)
	r.Get("/v1/assets/{id}", getAsset)
	log.Printf("vod2live api listening on %s", cfg.HTTPAddr)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, r))
}

var inMemoryAssets = map[uuid.UUID]asset.Asset{}

func createAsset(w http.ResponseWriter, r *http.Request) {
	var req struct { Title string `json:"title"`; HLSURL string `json:"hls_url"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.HLSURL) == "" {
		http.Error(w, "title e hls_url são obrigatórios", http.StatusBadRequest); return
	}
	item := asset.Asset{ID: uuid.New(), Title: req.Title, HLSURL: req.HLSURL, Status: asset.StatusPending}
	inMemoryAssets[item.ID] = item
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(item)
}

func getAsset(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id")); if err != nil { http.Error(w, "id inválido", 400); return }
	item, ok := inMemoryAssets[id]; if !ok { http.NotFound(w, r); return }
	w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(item)
}
