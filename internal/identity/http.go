package identity

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vod2live/cyonlive_vod2live/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	Repository   Repository
	Tokens       *auth.TokenService
	BootstrapKey string
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type bootstrapRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	OrganizationName string `json:"organization_name"`
}

type loginRequest struct {
	Email          string    `json:"email"`
	Password       string    `json:"password"`
	OrganizationID uuid.UUID `json:"organization_id"`
}

type tokenResponse struct {
	AccessToken    string    `json:"access_token"`
	TokenType      string    `json:"token_type"`
	ExpiresAt      time.Time `json:"expires_at"`
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Role           string    `json:"role"`
}

type organizationRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type teamRequest struct {
	Name string `json:"name"`
}

type teamMemberRequest struct {
	UserID uuid.UUID `json:"user_id"`
}

func (h Handler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	if h.BootstrapKey == "" {
		http.Error(w, "bootstrap desabilitado: configure API_KEY", http.StatusServiceUnavailable)
		return
	}
	var req bootstrapRequest
	if decodeJSON(w, r, &req) != nil || !validEmail(req.Email) || !validPassword(req.Password) || strings.TrimSpace(req.OrganizationName) == "" {
		http.Error(w, "email, senha de pelo menos 12 caracteres e organization_name são obrigatórios", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "não foi possível proteger a senha", http.StatusInternalServerError)
		return
	}
	userID, organizationID, err := h.Repository.Bootstrap(r.Context(), strings.ToLower(req.Email), string(hash), strings.TrimSpace(req.OrganizationName))
	if err != nil {
		if errors.Is(err, ErrBootstrapClosed) {
			http.Error(w, "bootstrap já concluído", http.StatusConflict)
		} else if h.Repository.IsUniqueViolation(err) {
			http.Error(w, "email já cadastrado", http.StatusConflict)
		} else {
			http.Error(w, "falha ao criar administrador inicial", http.StatusInternalServerError)
		}
		return
	}
	h.writeToken(w, http.StatusCreated, userID, organizationID, auth.RoleAdmin)
}

func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if decodeJSON(w, r, &req) != nil || !validEmail(req.Email) || req.Password == "" {
		http.Error(w, "email e senha são obrigatórios", http.StatusBadRequest)
		return
	}
	userID, hash, active, err := h.Repository.UserByEmail(r.Context(), req.Email)
	if err != nil || !active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		http.Error(w, "credenciais inválidas", http.StatusUnauthorized)
		return
	}
	memberships, err := h.Repository.Memberships(r.Context(), userID)
	if err != nil {
		http.Error(w, "falha ao consultar organizações", http.StatusInternalServerError)
		return
	}
	var selected *Organization
	for i := range memberships {
		if req.OrganizationID != uuid.Nil && memberships[i].ID == req.OrganizationID {
			selected = &memberships[i]
			break
		}
	}
	if req.OrganizationID == uuid.Nil && len(memberships) == 1 {
		selected = &memberships[0]
	}
	if selected == nil {
		if req.OrganizationID != uuid.Nil {
			http.Error(w, "credenciais inválidas", http.StatusUnauthorized)
		} else {
			http.Error(w, "organization_id é obrigatório quando o usuário pertence a várias organizações", http.StatusBadRequest)
		}
		return
	}
	h.writeToken(w, http.StatusOK, userID, selected.ID, selected.Role)
}

func (h Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}
	var req organizationRequest
	if decodeJSON(w, r, &req) != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name é obrigatório", http.StatusBadRequest)
		return
	}
	slug := slugify(req.Slug)
	if slug == "" {
		slug = slugify(req.Name)
	}
	if slug == "" {
		http.Error(w, "slug inválido", http.StatusBadRequest)
		return
	}
	item, err := h.Repository.CreateOrganization(r.Context(), principal.UserID, strings.TrimSpace(req.Name), slug)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}
	items, err := h.Repository.ListOrganizations(r.Context(), principal.UserID)
	if err != nil {
		http.Error(w, "falha ao listar organizações", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		http.Error(w, "não autorizado", http.StatusUnauthorized)
		return
	}
	var req createUserRequest
	if decodeJSON(w, r, &req) != nil || !validEmail(req.Email) || (req.Password != "" && !validPassword(req.Password)) || !auth.ValidRole(req.Role) {
		http.Error(w, "email e papel válido são obrigatórios; senha deve ter 12 a 72 bytes para um novo usuário", http.StatusBadRequest)
		return
	}
	var hash []byte
	var err error
	if req.Password != "" {
		hash, err = bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "não foi possível proteger a senha", http.StatusInternalServerError)
			return
		}
	}
	item, err := h.Repository.CreateUser(r.Context(), principal.OrganizationID, strings.ToLower(req.Email), string(hash), req.Role)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	items, err := h.Repository.ListUsers(r.Context(), principal.OrganizationID)
	if err != nil {
		http.Error(w, "falha ao listar usuários", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	var req struct {
		Role string `json:"role"`
	}
	if err != nil || decodeJSON(w, r, &req) != nil || !auth.ValidRole(req.Role) {
		http.Error(w, "id ou papel inválido", http.StatusBadRequest)
		return
	}
	if err = h.Repository.UpdateRole(r.Context(), principal.OrganizationID, userID, req.Role); err != nil {
		h.writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) RemoveUser(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if userID == principal.UserID {
		http.Error(w, "não é permitido remover a própria associação", http.StatusConflict)
		return
	}
	if err = h.Repository.RemoveUser(r.Context(), principal.OrganizationID, userID); err != nil {
		h.writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	var req teamRequest
	if decodeJSON(w, r, &req) != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name é obrigatório", http.StatusBadRequest)
		return
	}
	item, err := h.Repository.CreateTeam(r.Context(), principal.OrganizationID, strings.TrimSpace(req.Name))
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h Handler) ListTeams(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	items, err := h.Repository.ListTeams(r.Context(), principal.OrganizationID)
	if err != nil {
		http.Error(w, "falha ao listar equipes", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h Handler) AddTeamMember(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	teamID, err := uuid.Parse(chi.URLParam(r, "teamID"))
	var req teamMemberRequest
	if err != nil || decodeJSON(w, r, &req) != nil || req.UserID == uuid.Nil {
		http.Error(w, "team_id e user_id válidos são obrigatórios", http.StatusBadRequest)
		return
	}
	if err = h.Repository.AddTeamMember(r.Context(), principal.OrganizationID, teamID, req.UserID); err != nil {
		h.writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) RemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	teamID, err := uuid.Parse(chi.URLParam(r, "teamID"))
	userID, userErr := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil || userErr != nil {
		http.Error(w, "id inválido", http.StatusBadRequest)
		return
	}
	if err = h.Repository.RemoveTeamMember(r.Context(), principal.OrganizationID, teamID, userID); err != nil {
		h.writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) writeToken(w http.ResponseWriter, status int, userID, organizationID uuid.UUID, role string) {
	token, expires, err := h.Tokens.Issue(userID, organizationID)
	if err != nil {
		http.Error(w, "não foi possível criar sessão", http.StatusInternalServerError)
		return
	}
	writeJSON(w, status, tokenResponse{AccessToken: token, TokenType: "Bearer", ExpiresAt: expires.UTC(), UserID: userID, OrganizationID: organizationID, Role: role})
}

func (h Handler) writeRepoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "recurso não encontrado nesta organização", http.StatusNotFound)
	case errors.Is(err, ErrLastAdmin):
		http.Error(w, ErrLastAdmin.Error(), http.StatusConflict)
	case errors.Is(err, ErrAlreadyMember):
		http.Error(w, "usuário já é membro desta organização", http.StatusConflict)
	case errors.Is(err, ErrPasswordRequired):
		http.Error(w, "senha de pelo menos 12 caracteres obrigatória para novo usuário", http.StatusBadRequest)
	case h.Repository.IsUniqueViolation(err):
		http.Error(w, "já existe um registro com estes dados", http.StatusConflict)
	case h.Repository.IsForeignKeyViolation(err):
		http.Error(w, "recurso não encontrado nesta organização", http.StatusNotFound)
	default:
		http.Error(w, "operação de identidade não concluída", http.StatusInternalServerError)
	}
}

func AuditMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	repo := Repository{Pool: pool}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tracked := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(tracked, r)
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				return
			}
			principal, ok := auth.PrincipalFrom(r.Context())
			if !ok {
				return
			}
			if err := repo.Audit(r.Context(), principal.OrganizationID, principal.UserID, r.Method, r.URL.Path, tracked.status, r.Header.Get("X-Request-ID")); err != nil {
				log.Printf("audit insert failed: %v", err)
			}
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(status int) {
	if !w.wrote {
		w.status = status
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func validEmail(value string) bool {
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	return err == nil && strings.EqualFold(address.Address, strings.TrimSpace(value)) && len(value) <= 254
}

func validPassword(value string) bool {
	return utf8.RuneCountInString(value) >= 12 && len(value) <= 72
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
