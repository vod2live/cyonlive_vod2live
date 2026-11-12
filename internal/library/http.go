package library

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/internal/auth"
	"github.com/vod2live/cyonlive_vod2live/internal/storage"
)

type Handler struct {
	Repo    Repository
	Storage *storage.Client
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
	write(w, 200, items)
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
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Filename == "" || req.SizeBytes <= 0 {
		http.Error(w, "filename, content_type e size_bytes são obrigatórios", 400)
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
