package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/auth"
	"github.com/vod2live/cyonlive_vod2live/internal/buildinfo"
	"github.com/vod2live/cyonlive_vod2live/internal/channel"
	"github.com/vod2live/cyonlive_vod2live/internal/config"
	"github.com/vod2live/cyonlive_vod2live/internal/db"
	"github.com/vod2live/cyonlive_vod2live/internal/health"
	"github.com/vod2live/cyonlive_vod2live/internal/identity"
	"github.com/vod2live/cyonlive_vod2live/internal/library"
	"github.com/vod2live/cyonlive_vod2live/internal/manifest"
	"github.com/vod2live/cyonlive_vod2live/internal/metrics"
	"github.com/vod2live/cyonlive_vod2live/internal/observability"
	"github.com/vod2live/cyonlive_vod2live/internal/playback"
	"github.com/vod2live/cyonlive_vod2live/internal/playout"
	"github.com/vod2live/cyonlive_vod2live/internal/ratelimit"
	"github.com/vod2live/cyonlive_vod2live/internal/retry"
	"github.com/vod2live/cyonlive_vod2live/internal/schedule"
	"github.com/vod2live/cyonlive_vod2live/internal/security"
	"github.com/vod2live/cyonlive_vod2live/internal/storage"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := retry.Run(context.Background(), 8, 500*time.Millisecond, func() error { return redisClient.Ping(context.Background()).Err() }); err != nil {
		log.Fatalf("redis: %v", err)
	}
	assets := asset.PostgresRepository{Pool: pool}
	mediaCfg := config.LoadMedia()
	objectStore, err := storage.New(mediaCfg.StorageEndpoint, mediaCfg.StorageAccessKey, mediaCfg.StorageSecretKey, mediaCfg.StorageBucket, mediaCfg.StorageUseSSL)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	identities := identity.Repository{Pool: pool}
	libraryHandler := library.Handler{Repo: library.Repository{Pool: pool}, Storage: objectStore, TenantAccess: identities}
	channels := channel.PostgresRepository{Pool: pool}
	schedules := schedule.PostgresRepository{Pool: pool}
	manifests := manifest.Service{Channels: channels, Assets: assets, Schedules: schedules, Cache: manifest.ManifestCache{Client: redisClient}, Storage: objectStore}
	playbackSigner, err := playback.NewSigner(cfg.PlaybackTokenSecret, time.Duration(cfg.PlaybackTokenTTLSeconds)*time.Second)
	if err != nil {
		log.Fatalf("playback configuration: %v", err)
	}
	playbackHandler := playback.Handler{Signer: playbackSigner, Manifests: manifests, AllowPrivate: cfg.AllowPrivateHLS, CORSOrigin: cfg.CORSAllowedOrigin}
	sessionTokens, err := auth.NewTokenService(cfg.AuthTokenSecret, time.Duration(cfg.AuthTokenTTLSeconds)*time.Second)
	if err != nil {
		log.Fatalf("auth configuration: %v", err)
	}
	identityHandler := identity.Handler{Repository: identities, Tokens: sessionTokens, BootstrapKey: cfg.APIKey}
	playoutStore := playout.RedisStore{Client: redisClient}
	apiMetrics := &metrics.Metrics{}
	limiter := ratelimit.New(cfg.RateLimitPerMinute)
	r := chi.NewRouter()
	r.Use(observability.Middleware(logger))
	r.Use(security.Headers)
	r.Use(apiMetrics.Middleware)
	r.Get("/healthz", health.Handler)
	r.Get("/readyz", health.ReadyHandler(pool, redisClient))
	r.Get("/metrics", apiMetrics.Handler)
	r.Get("/version", buildinfo.Handler)
	r.Route("/playback/{channelID}", func(play chi.Router) {
		play.Get("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(chi.URLParam(r, "channelID"))
			if err != nil {
				http.Error(w, "id inválido", http.StatusBadRequest)
				return
			}
			playbackHandler.ServeMaster(w, r, id)
		})
		play.Get("/media/{variant}.m3u8", func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(chi.URLParam(r, "channelID"))
			if err != nil {
				http.Error(w, "id inválido", http.StatusBadRequest)
				return
			}
			playbackHandler.ServeMedia(w, r, id, chi.URLParam(r, "variant"))
		})
		play.Get("/segment", func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(chi.URLParam(r, "channelID"))
			if err != nil {
				http.Error(w, "id inválido", http.StatusBadRequest)
				return
			}
			playbackHandler.ServeSegment(w, r, id)
		})
	})
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(limiter.Middleware)
		v1.With(auth.APIKey(cfg.APIKey)).Post("/auth/bootstrap", identityHandler.Bootstrap)
		v1.Post("/auth/login", identityHandler.Login)
		v1.Group(func(api chi.Router) {
			api.Use(auth.Bearer(sessionTokens, identities))
			api.Use(identity.AuditMiddleware(pool))
			api.Get("/organizations", identityHandler.ListOrganizations)
			api.Post("/organizations", identityHandler.CreateOrganization)
			api.Get("/tenants", libraryHandler.ListTenants)
			api.With(auth.RequireOperation(auth.OperationManageTenants)).Post("/tenants", libraryHandler.CreateTenant)
			api.With(auth.RequireOperation(auth.OperationManageTenants)).Post("/tenants/{tenantID}/members", libraryHandler.AddTenantMember)
			api.With(auth.RequireOperation(auth.OperationManageTenants)).Delete("/tenants/{tenantID}/members/{userID}", libraryHandler.RemoveTenantMember)
			api.Get("/libraries", libraryHandler.List)
			api.Get("/uploads", libraryHandler.ListUploads)
			api.Get("/media/jobs", libraryHandler.JobStats)
			api.With(auth.RequireOperation(auth.OperationWriteLibraries)).Post("/libraries", libraryHandler.Create)
			api.Get("/storage/quota", libraryHandler.Quota)
			api.With(auth.RequireOperation(auth.OperationWriteQuota)).Post("/storage/quota", libraryHandler.SetQuota)
			api.With(auth.RequireOperation(auth.OperationWriteLibraries)).Post("/libraries/{libraryID}/uploads", libraryHandler.CreateUpload)
			api.With(auth.RequireOperation(auth.OperationWriteLibraries)).Post("/uploads/{uploadID}/complete", libraryHandler.CompleteUpload)
			api.With(auth.RequireOperation(auth.OperationManageUsers)).Get("/users", identityHandler.ListUsers)
			api.With(auth.RequireOperation(auth.OperationManageUsers)).Post("/users", identityHandler.CreateUser)
			api.With(auth.RequireOperation(auth.OperationManageUsers)).Patch("/users/{userID}/role", identityHandler.UpdateUserRole)
			api.With(auth.RequireOperation(auth.OperationManageUsers)).Delete("/users/{userID}", identityHandler.RemoveUser)
			api.Get("/teams", identityHandler.ListTeams)
			api.With(auth.RequireOperation(auth.OperationManageTeams)).Post("/teams", identityHandler.CreateTeam)
			api.With(auth.RequireOperation(auth.OperationManageTeams)).Post("/teams/{teamID}/members", identityHandler.AddTeamMember)
			api.With(auth.RequireOperation(auth.OperationManageTeams)).Delete("/teams/{teamID}/members/{userID}", identityHandler.RemoveTeamMember)

			api.With(auth.RequireOperation(auth.OperationWriteAssets)).Post("/assets", createAsset(assets))
			api.With(auth.RequireOperation(auth.OperationReviewAssets)).Get("/review/assets", reviewQueue(assets))
			api.With(auth.RequireOperation(auth.OperationReviewAssets)).Post("/assets/{id}/review", reviewAsset(assets, identities))
			api.With(auth.RequireOperation(auth.OperationReviewAssets)).Get("/assets/{id}/review/history", reviewHistory(assets, identities))
			api.Get("/assets/{id}", getAsset(assets, identities))
			api.With(auth.RequireOperation(auth.OperationValidateAssets)).Post("/assets/{id}/validate", validateAsset(assets, identities))
			api.With(auth.RequireOperation(auth.OperationWriteChannels)).Post("/channels", createChannel(channels, identities))
			api.Get("/channels", listChannels(channels))
			api.Get("/channels/{id}", getChannel(channels, identities))
			api.With(auth.RequireOperation(auth.OperationControlChannels)).Post("/channels/{id}/start", updateChannelStatus(channels, identities, "running"))
			api.With(auth.RequireOperation(auth.OperationControlChannels)).Post("/channels/{id}/stop", updateChannelStatus(channels, identities, "stopped"))
			api.Get("/channels/{id}/status", channelStatus(channels, identities, playoutStore))
			api.Get("/channels/{id}/master.m3u8", masterManifest(channels, identities, manifests))
			api.Get("/channels/{id}/media/{variant}.m3u8", mediaManifest(channels, identities, manifests))
			api.With(auth.RequireOperation(auth.OperationIssuePlayback)).Post("/channels/{id}/playback", issuePlaybackLink(channels, identities, playbackSigner))
			api.With(auth.RequireOperation(auth.OperationWriteSchedule)).Post("/channels/{id}/schedule", createScheduleItem(channels, assets, identities, schedules))
			api.Get("/channels/{id}/schedule", listSchedule(channels, identities, schedules))
			api.With(auth.RequireOperation(auth.OperationWriteSchedule)).Patch("/schedule/{id}", updateScheduleItem(identities, schedules))
			api.With(auth.RequireOperation(auth.OperationWriteSchedule)).Delete("/schedule/{id}", deleteScheduleItem(identities, schedules))
		})
	})
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: r, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	serverErr := make(chan error, 1)
	go func() { log.Printf("vod2live api listening on %s", cfg.HTTPAddr); serverErr <- server.ListenAndServe() }()
	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-stopCtx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown: %v", err)
	}
}

type assetRequest struct {
	Title  string `json:"title"`
	HLSURL string `json:"hls_url"`
}

func createAsset(repo asset.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		var req assetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.HLSURL) == "" {
			http.Error(w, "title e hls_url são obrigatórios", 400)
			return
		}
		item := asset.Asset{ID: uuid.New(), OrganizationID: principal.OrganizationID, Title: req.Title, HLSURL: strings.TrimSpace(req.HLSURL), Status: asset.StatusPending}
		if err := repo.Create(r.Context(), principal.OrganizationID, item); err != nil {
			http.Error(w, "erro ao criar asset", 500)
			return
		}
		writeJSON(w, 201, item)
	}
}
func getAsset(repo asset.PostgresRepository, access identity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		if allowed, accessErr := access.UserCanAccessAsset(r.Context(), principal.UserID, principal.OrganizationID, id); accessErr != nil || !allowed {
			http.Error(w, "asset não encontrado", http.StatusNotFound)
			return
		}
		item, err := repo.FindByIDInOrg(r.Context(), principal.OrganizationID, id)
		if err != nil {
			http.Error(w, "asset não encontrado", 404)
			return
		}
		writeJSON(w, 200, item)
	}
}
func validateAsset(repo asset.PostgresRepository, access identity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		if allowed, accessErr := access.UserCanAccessAsset(r.Context(), principal.UserID, principal.OrganizationID, id); accessErr != nil || !allowed {
			http.Error(w, "asset não encontrado", http.StatusNotFound)
			return
		}
		item, err := repo.FindByIDInOrg(r.Context(), principal.OrganizationID, id)
		if err != nil {
			http.Error(w, "asset não encontrado", 404)
			return
		}
		cfg := config.Load()
		inspection, err := (asset.HLSInspector{AllowPrivate: cfg.AllowPrivateHLS, MaxManifestBytes: cfg.HLSMaxManifestBytes, Timeout: time.Duration(cfg.HLSRequestTimeoutSecs) * time.Second}).Inspect(item.HLSURL)
		if err != nil {
			_ = repo.UpdateStatusInOrg(r.Context(), principal.OrganizationID, id, asset.StatusInvalid, 0, map[string]string{"error": err.Error()})
			http.Error(w, err.Error(), 422)
			return
		}
		_ = repo.UpdateStatusInOrg(r.Context(), principal.OrganizationID, id, asset.StatusValid, inspection.DurationMS, inspection)
		writeJSON(w, 200, inspection)
	}
}

func reviewQueue(repo asset.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
			items, err := repo.ListAwaitingReviewForUser(r.Context(), principal.UserID, principal.OrganizationID)
		if err != nil {
			http.Error(w, "erro ao consultar fila de revisão", http.StatusInternalServerError)
			return
		}
			writeJSON(w, http.StatusOK, items)
	}
}

func reviewAsset(repo asset.PostgresRepository, access identity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", http.StatusBadRequest)
			return
		}
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		if allowed, accessErr := access.UserCanAccessAsset(r.Context(), principal.UserID, principal.OrganizationID, id); accessErr != nil || !allowed {
			http.Error(w, "asset não encontrado", http.StatusNotFound)
			return
		}
		var req struct {
			Decision string `json:"decision"`
			Note     string `json:"note"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Decision != "approve" && req.Decision != "reject") {
			http.Error(w, "decision deve ser approve ou reject", http.StatusBadRequest)
			return
		}
		err = repo.Review(r.Context(), principal.OrganizationID, id, principal.UserID, req.Decision == "approve", req.Note)
		if errors.Is(err, asset.ErrReviewNotAllowed) {
			http.Error(w, "asset não está pronto para revisão ou já foi decidido", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "erro ao registrar revisão", http.StatusInternalServerError)
			return
		}
		status := asset.ApprovalRejected
		if req.Decision == "approve" {
			status = asset.ApprovalApproved
		}
		writeJSON(w, http.StatusOK, map[string]any{"asset_id": id, "approval_status": status, "reviewed_by": principal.UserID, "note": strings.TrimSpace(req.Note)})
	}
}

func reviewHistory(repo asset.PostgresRepository, access identity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", http.StatusBadRequest)
			return
		}
		if allowed, accessErr := access.UserCanAccessAsset(r.Context(), principal.UserID, principal.OrganizationID, id); accessErr != nil || !allowed {
			http.Error(w, "asset não encontrado", http.StatusNotFound)
			return
		}
		if _, err = repo.FindByIDInOrg(r.Context(), principal.OrganizationID, id); err != nil {
			http.Error(w, "asset não encontrado", http.StatusNotFound)
			return
		}
		history, err := repo.ReviewHistory(r.Context(), principal.OrganizationID, id)
		if err != nil {
			http.Error(w, "erro ao consultar histórico editorial", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, history)
	}
}

type channelRequest struct {
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	LoopEnabled bool       `json:"loop_enabled"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
}

func createChannel(repo channel.PostgresRepository, tenantAccess interface {
	UserHasTenant(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		var req channelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			http.Error(w, "name é obrigatório", 400)
			return
		}
		if req.TenantID != nil {
			allowed, accessErr := tenantAccess.UserHasTenant(r.Context(), principal.UserID, principal.OrganizationID, *req.TenantID)
			if accessErr != nil {
				http.Error(w, "falha ao validar tenant", http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, "tenant não autorizado", http.StatusForbidden)
				return
			}
		}
		slug := channel.NormalizeSlug(req.Slug)
		if slug == "" {
			slug = channel.NormalizeSlug(req.Name)
		}
		item := channel.Channel{ID: uuid.New(), OrganizationID: principal.OrganizationID, TenantID: req.TenantID, Name: req.Name, Slug: slug, Status: "draft", LoopEnabled: req.LoopEnabled}
		if err := repo.Create(r.Context(), item); err != nil {
			http.Error(w, "erro ao criar canal", 500)
			return
		}
		writeJSON(w, 201, item)
	}
}
func checkChannelAccess(w http.ResponseWriter, r *http.Request, access identity.Repository, principal auth.Principal, channelID uuid.UUID) bool {
	allowed, err := access.UserCanAccessChannel(r.Context(), principal.UserID, principal.OrganizationID, channelID)
	if err != nil {
		http.Error(w, "falha ao validar acesso ao canal", http.StatusInternalServerError)
		return false
	}
	if !allowed {
		http.Error(w, "canal não encontrado", http.StatusNotFound)
		return false
	}
	return true
}

func listChannels(repo channel.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
			items, err := repo.ListVisibleToUser(r.Context(), principal.UserID, principal.OrganizationID)
		if err != nil {
			http.Error(w, "erro ao listar canais", 500)
			return
		}
			writeJSON(w, 200, items)
	}
}
func getChannel(repo channel.PostgresRepository, access identity.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		if !checkChannelAccess(w, r, access, principal, id) {
			return
		}
		item, err := repo.FindByIDInOrg(r.Context(), principal.OrganizationID, id)
		if err != nil {
			http.Error(w, "canal não encontrado", 404)
			return
		}
		writeJSON(w, 200, item)
	}
}

func updateChannelStatus(repo channel.PostgresRepository, access identity.Repository, status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		if !checkChannelAccess(w, r, access, principal, id) {
			return
		}
		if _, err = repo.FindByIDInOrg(r.Context(), principal.OrganizationID, id); err != nil {
			http.Error(w, "canal não encontrado", 404)
			return
		}
		if err = repo.UpdateStatusInOrg(r.Context(), principal.OrganizationID, id, status); err != nil {
			http.Error(w, "erro ao atualizar canal", 500)
			return
		}
		writeJSON(w, 200, map[string]string{"channel_id": id.String(), "status": status})
	}
}

func channelStatus(repo channel.PostgresRepository, access identity.Repository, store playout.RedisStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		if !checkChannelAccess(w, r, access, principal, id) {
			return
		}
		item, err := repo.FindByIDInOrg(r.Context(), principal.OrganizationID, id)
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

func createScheduleItem(channels channel.PostgresRepository, assets asset.PostgresRepository, access identity.Repository, repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id de canal inválido", 400)
			return
		}
		if !checkChannelAccess(w, r, access, principal, channelID) {
			return
		}
		channelItem, findErr := channels.FindByIDInOrg(r.Context(), principal.OrganizationID, channelID)
		if findErr != nil {
			http.Error(w, "canal não encontrado", 404)
			return
		}
		var req scheduleRequest
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil || req.AssetID == uuid.Nil || req.StartsAt.IsZero() {
			http.Error(w, "asset_id e starts_at são obrigatórios", 400)
			return
		}
		assetItem, err := assets.FindByIDInOrg(r.Context(), principal.OrganizationID, req.AssetID)
		if err != nil || !asset.EligibleForChannel(assetItem, channelItem.TenantID) {
			http.Error(w, "asset precisa estar aprovado e pertencer ao escopo do canal", 422)
			return
		}
		if req.EndsAt == nil && assetItem.DurationMS > 0 {
			derived := req.StartsAt.Add(time.Duration(assetItem.DurationMS) * time.Millisecond)
			req.EndsAt = &derived
		}
		item := schedule.Item{ID: uuid.New(), OrganizationID: principal.OrganizationID, ChannelID: channelID, AssetID: req.AssetID, StartsAt: req.StartsAt, EndsAt: req.EndsAt, Position: req.Position, Enabled: true}
		if err := repo.Create(r.Context(), item); err != nil {
			if errors.Is(err, context.Canceled) {
				http.Error(w, "requisição cancelada", 499)
			} else if errors.Is(err, schedule.ErrAssetUnavailable) {
				http.Error(w, "asset não aprovado ou fora do escopo do canal", 422)
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
func listSchedule(channels channel.PostgresRepository, access identity.Repository, repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		if !checkChannelAccess(w, r, access, principal, id) {
			return
		}
		if _, err := channels.FindByIDInOrg(r.Context(), principal.OrganizationID, id); err != nil {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
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

func updateScheduleItem(access identity.Repository, repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		if allowed, accessErr := access.UserCanAccessSchedule(r.Context(), principal.UserID, principal.OrganizationID, id); accessErr != nil || !allowed {
			http.Error(w, "item não encontrado", http.StatusNotFound)
			return
		}
		var req scheduleUpdateRequest
		if err = json.NewDecoder(r.Body).Decode(&req); err != nil || req.StartsAt.IsZero() {
			http.Error(w, "starts_at é obrigatório", 400)
			return
		}
		if err = repo.Update(r.Context(), principal.OrganizationID, id, req.StartsAt, req.EndsAt, req.Position, req.Enabled); err != nil {
			if errors.Is(err, schedule.ErrNotFound) {
				http.Error(w, "item não encontrado", http.StatusNotFound)
			} else if errors.Is(err, schedule.ErrAssetUnavailable) {
				http.Error(w, "asset não aprovado ou fora do escopo do canal", 422)
			} else if strings.Contains(err.Error(), "schedule conflict") {
				http.Error(w, "conflito de horário na grade", 409)
			} else {
				http.Error(w, "erro ao atualizar item da grade", 500)
			}
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "starts_at": req.StartsAt, "ends_at": req.EndsAt, "position": req.Position, "enabled": req.Enabled})
	}
}

func deleteScheduleItem(access identity.Repository, repo schedule.PostgresRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		if allowed, accessErr := access.UserCanAccessSchedule(r.Context(), principal.UserID, principal.OrganizationID, id); accessErr != nil || !allowed {
			http.Error(w, "item não encontrado", http.StatusNotFound)
			return
		}
		if err = repo.Delete(r.Context(), principal.OrganizationID, id); err != nil {
			if errors.Is(err, schedule.ErrNotFound) {
				http.Error(w, "item não encontrado", http.StatusNotFound)
				return
			}
			http.Error(w, "erro ao remover item da grade", 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func masterManifest(channels channel.PostgresRepository, access identity.Repository, service manifest.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		if !checkChannelAccess(w, r, access, principal, id) {
			return
		}
		if _, err := channels.FindByIDInOrg(r.Context(), principal.OrganizationID, id); err != nil {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
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
func mediaManifest(channels channel.PostgresRepository, access identity.Repository, service manifest.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", 400)
			return
		}
		if !checkChannelAccess(w, r, access, principal, id) {
			return
		}
		if _, err := channels.FindByIDInOrg(r.Context(), principal.OrganizationID, id); err != nil {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
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

func issuePlaybackLink(repo channel.PostgresRepository, access identity.Repository, signer *playback.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "id inválido", http.StatusBadRequest)
			return
		}
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		if !checkChannelAccess(w, r, access, principal, id) {
			return
		}
		if _, err = repo.FindByIDInOrg(r.Context(), principal.OrganizationID, id); err != nil {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
			return
		}
		now := time.Now().UTC()
		expires, token := signer.NewToken(id, "master", now)
		url := fmt.Sprintf("/playback/%s/master.m3u8?expires=%d&token=%s", id, expires, token)
		writeJSON(w, http.StatusCreated, map[string]any{
			"channel_id":   id,
			"playback_url": url,
			"expires_at":   time.Unix(expires, 0).UTC(),
		})
	}
}
