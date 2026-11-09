package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const issuer = "vod2live"

type Principal struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Role           string    `json:"role"`
}

type tokenClaims struct {
	Subject        string `json:"sub"`
	OrganizationID string `json:"org"`
	IssuedAt       int64  `json:"iat"`
	ExpiresAt      int64  `json:"exp"`
	Issuer         string `json:"iss"`
}

type TokenService struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenService(secret string, ttl time.Duration) (*TokenService, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("AUTH_TOKEN_SECRET deve conter ao menos 32 bytes")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("AUTH_TOKEN_TTL_SECONDS deve ser positivo")
	}
	return &TokenService{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

func (s *TokenService) Issue(userID, organizationID uuid.UUID) (string, time.Time, error) {
	now := s.now().UTC()
	expires := now.Add(s.ttl)
	claims := tokenClaims{Subject: userID.String(), OrganizationID: organizationID.String(), IssuedAt: now.Unix(), ExpiresAt: expires.Unix(), Issuer: issuer}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), expires, nil
}

func (s *TokenService) Verify(raw string) (Principal, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Principal{}, fmt.Errorf("malformed bearer token")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || string(header) != `{"alg":"HS256","typ":"JWT"}` {
		return Principal{}, fmt.Errorf("unsupported token header")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Principal{}, fmt.Errorf("invalid token signature")
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return Principal{}, fmt.Errorf("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, fmt.Errorf("invalid token payload")
	}
	var claims tokenClaims
	if err = json.Unmarshal(payload, &claims); err != nil || claims.Issuer != issuer || claims.ExpiresAt <= s.now().UTC().Unix() || claims.IssuedAt > s.now().UTC().Add(time.Minute).Unix() {
		return Principal{}, fmt.Errorf("expired or invalid token claims")
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Principal{}, fmt.Errorf("invalid token subject")
	}
	organizationID, err := uuid.Parse(claims.OrganizationID)
	if err != nil {
		return Principal{}, fmt.Errorf("invalid token organization")
	}
	return Principal{UserID: userID, OrganizationID: organizationID}, nil
}

type MembershipLookup interface {
	RoleForUser(context.Context, uuid.UUID, uuid.UUID) (string, error)
}

type principalContextKey struct{}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func Bearer(tokens *TokenService, memberships MembershipLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				authFailure()
				http.Error(w, "não autorizado", http.StatusUnauthorized)
				return
			}
			principal, err := tokens.Verify(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
			if err != nil {
				authFailure()
				http.Error(w, "não autorizado", http.StatusUnauthorized)
				return
			}
			role, err := memberships.RoleForUser(r.Context(), principal.UserID, principal.OrganizationID)
			if err != nil || !ValidRole(role) {
				authFailure()
				http.Error(w, "não autorizado", http.StatusUnauthorized)
				return
			}
			principal.Role = role
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
		})
	}
}

const (
	RoleAdmin    = "administrador"
	RoleOperator = "operador"
	RoleEditor   = "editor"
	RoleViewer   = "visualizador"
)

func ValidRole(role string) bool {
	return role == RoleAdmin || role == RoleOperator || role == RoleEditor || role == RoleViewer
}

func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFrom(r.Context())
			if !ok {
				http.Error(w, "não autorizado", http.StatusUnauthorized)
				return
			}
			if _, ok = allowed[principal.Role]; !ok {
				http.Error(w, "permissão insuficiente", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func authFailure() { recordAuthFailure() }
