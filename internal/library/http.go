package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/internal/auth"
	"github.com/vod2live/cyonlive_vod2live/internal/storage"
)

type Handler struct {
	Repo         Repository
	Storage      *storage.Client
	TenantAccess TenantAuthorizer
}

type TenantAuthorizer interface {
	UserHasTenant(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
}

func (h Handler) authorizeTenant(ctx context.Context, p auth.Principal, tenant *uuid.UUID) error {
	if tenant == nil {
		return nil
	}
	if h.TenantAccess == nil {
		return fmt.Errorf("tenant authorization is not configured")
	}
	ok, err := h.TenantAccess.UserHasTenant(ctx, p.UserID, p.OrganizationID, *tenant)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("tenant access denied")
	}
	return nil
}

func (h Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	items, err := h.Repo.ListTenants(r.Context(), p.OrganizationID)
	if err != nil {
		http.Error(w, "erro ao listar tenants", 500)
		return
	}
	write(w, 200, items)
}

func (h Handler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "payload inválido", 400)
		return
	}
	item, err := h.Repo.CreateTenant(r.Context(), p.OrganizationID, req.Name, req.Slug)
	if err != nil {
		http.Error(w, "erro ao criar tenant", 422)
		return
	}
	if err := h.Repo.AddTenantMember(r.Context(), p.OrganizationID, item.ID, p.UserID, auth.RoleAdmin); err != nil {
		http.Error(w, "erro ao associar administrador ao tenant", http.StatusInternalServerError)
		return
	}
	write(w, 201, item)
}

func (h Handler) SetQuota(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	var req struct {
		TenantID   *uuid.UUID `json:"tenant_id"`
		QuotaBytes int64      `json:"quota_bytes"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.QuotaBytes < 0 {
		http.Error(w, "quota_bytes é obrigatório e não pode ser negativa", 400)
		return
	}
	if err := h.authorizeTenant(r.Context(), p, req.TenantID); err != nil {
		http.Error(w, "tenant não autorizado", http.StatusForbidden)
		return
	}
	if err := h.Repo.EnsureQuota(r.Context(), p.OrganizationID, req.TenantID, req.QuotaBytes); err != nil {
		http.Error(w, "erro ao configurar quota", 422)
		return
	}
	q, err := h.Repo.Quota(r.Context(), p.OrganizationID, req.TenantID)
	if err != nil {
		http.Error(w, "quota não encontrada", 500)
		return
	}
	write(w, 200, q)
}

func (h Handler) AddTenantMember(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		http.Error(w, "tenant_id inválido", http.StatusBadRequest)
		return
	}
	var req struct {
		UserID uuid.UUID `json:"user_id"`
		Role   string    `json:"role"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.UserID == uuid.Nil {
		http.Error(w, "user_id é obrigatório", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "membro"
	}
	if req.Role != "administrador" && req.Role != "editor" && req.Role != "operador" && req.Role != "visualizador" && req.Role != "membro" {
		http.Error(w, "role inválido", http.StatusBadRequest)
		return
	}
	if err := h.Repo.AddTenantMember(r.Context(), p.OrganizationID, tenantID, req.UserID, req.Role); err != nil {
		http.Error(w, "usuário ou tenant inválido", http.StatusUnprocessableEntity)
		return
	}
	write(w, http.StatusCreated, map[string]any{"tenant_id": tenantID, "user_id": req.UserID, "role": req.Role})
}

func (h Handler) RemoveTenantMember(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	userID, userErr := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil || userErr != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if userID == p.UserID {
		http.Error(w, "não é permitido remover a própria associação", http.StatusConflict)
		return
	}
	if err := h.Repo.RemoveTenantMember(r.Context(), p.OrganizationID, tenantID, userID); err != nil {
		http.Error(w, "membro não encontrado", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	items, err := h.Repo.ListLibraries(r.Context(), p.OrganizationID)
	if err != nil {
		http.Error(w, "erro ao listar bibliotecas", 500)
		return
	}
	filtered := items[:0]
	for _, item := range items {
		if item.TenantID == nil || h.authorizeTenant(r.Context(), p, item.TenantID) == nil {
			filtered = append(filtered, item)
		}
	}
	write(w, 200, filtered)
}
func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	var req struct {
		Name     string     `json:"name"`
		TenantID *uuid.UUID `json:"tenant_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name é obrigatório", 400)
		return
	}
	if err := h.authorizeTenant(r.Context(), p, req.TenantID); err != nil {
		http.Error(w, "tenant não autorizado", http.StatusForbidden)
		return
	}
	item, err := h.Repo.CreateLibrary(r.Context(), p.OrganizationID, req.TenantID, req.Name)
	if err != nil {
		http.Error(w, "erro ao criar biblioteca", 500)
		return
	}
	write(w, 201, item)
}
func (h Handler) Quota(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	var tenant *uuid.UUID
	if raw := r.URL.Query().Get("tenant_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "tenant_id inválido", 400)
			return
		}
		tenant = &id
	}
	if err := h.authorizeTenant(r.Context(), p, tenant); err != nil {
		http.Error(w, "tenant não autorizado", http.StatusForbidden)
		return
	}
	q, err := h.Repo.Quota(r.Context(), p.OrganizationID, tenant)
	if err != nil {
		http.Error(w, "quota não configurada", 404)
		return
	}
	write(w, 200, q)
}
func (h Handler) CreateUpload(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	libraryID, err := uuid.Parse(chi.URLParam(r, "libraryID"))
	if err != nil {
		http.Error(w, "biblioteca inválida", 400)
		return
	}
	var req struct {
		Filename    string     `json:"filename"`
		ContentType string     `json:"content_type"`
		SizeBytes   int64      `json:"size_bytes"`
		TenantID    *uuid.UUID `json:"tenant_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Filename == "" || req.SizeBytes <= 0 || req.SizeBytes > MaxUploadBytes {
		http.Error(w, "filename, content_type e size_bytes são obrigatórios", 400)
		return
	}
	if !acceptedUploadType(req.ContentType) {
		http.Error(w, "content_type não suportado", http.StatusUnsupportedMediaType)
		return
	}
	if err := h.authorizeTenant(r.Context(), p, req.TenantID); err != nil {
		http.Error(w, "tenant não autorizado", http.StatusForbidden)
		return
	}
	u, err := h.Repo.ReserveUpload(r.Context(), p.OrganizationID, req.TenantID, libraryID, req.Filename, req.ContentType, req.SizeBytes, h.Storage.Bucket)
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	put, err := h.Storage.PresignedPut(r.Context(), u.ObjectKey, 30*time.Minute)
	if err != nil {
		http.Error(w, "erro ao gerar upload", 500)
		return
	}
	write(w, 201, map[string]any{"upload": u, "upload_url": put.String(), "expires_at": time.Now().UTC().Add(30 * time.Minute)})
}

func acceptedUploadType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "video/mp4", "video/quicktime", "video/x-matroska", "video/webm", "application/octet-stream":
		return true
	default:
		return false
	}
}

func (h Handler) ListUploads(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}
	var libraryID *uuid.UUID
	if raw := r.URL.Query().Get("library_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "library_id inválido", http.StatusBadRequest)
			return
		}
		libraryID = &id
	}
	items, err := h.Repo.ListUploads(r.Context(), p.OrganizationID, libraryID)
	if err != nil {
		http.Error(w, "erro ao listar uploads", http.StatusInternalServerError)
		return
	}
	filtered := items[:0]
	for _, item := range items {
		if item.TenantID == nil || h.authorizeTenant(r.Context(), p, item.TenantID) == nil {
			filtered = append(filtered, item)
		}
	}
	write(w, http.StatusOK, filtered)
}

func (h Handler) JobStats(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}
	var tenant *uuid.UUID
	if raw := r.URL.Query().Get("tenant_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "tenant_id inválido", http.StatusBadRequest)
			return
		}
		tenant = &id
	}
	if err := h.authorizeTenant(r.Context(), p, tenant); err != nil {
		http.Error(w, "tenant não autorizado", http.StatusForbidden)
		return
	}
	stats, err := h.Repo.JobStats(r.Context(), p.OrganizationID, tenant)
	if err != nil {
		http.Error(w, "erro ao consultar jobs", http.StatusInternalServerError)
		return
	}
	write(w, http.StatusOK, stats)
}

func (h Handler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", 401)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "uploadID"))
	if err != nil {
		http.Error(w, "upload inválido", 400)
		return
	}
	var req struct {
		SizeBytes int64 `json:"size_bytes"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.SizeBytes <= 0 {
		http.Error(w, "size_bytes é obrigatório", 400)
		return
	}
	tenant, err := h.Repo.UploadTenant(r.Context(), p.OrganizationID, id)
	if err != nil {
		http.Error(w, "upload não encontrado", http.StatusNotFound)
		return
	}
	if err := h.authorizeTenant(r.Context(), p, tenant); err != nil {
		http.Error(w, "tenant não autorizado", http.StatusForbidden)
		return
	}
	actual, _, err := h.Storage.Stat(r.Context(), func() string {
		u, e := h.Repo.UploadKey(r.Context(), p.OrganizationID, id)
		if e != nil {
			return ""
		}
		return u
	}())
	if err != nil {
		http.Error(w, "objeto não encontrado no storage", 422)
		return
	}
	if req.SizeBytes != actual {
		http.Error(w, "tamanho informado não corresponde ao objeto", 422)
		return
	}
	u, err := h.Repo.CompleteUpload(r.Context(), p.OrganizationID, id, actual)
	if err != nil {
		http.Error(w, "erro ao concluir upload", 422)
		return
	}
	write(w, 200, u)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
