package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

const authTestSecret = "a-testing-secret-that-is-at-least-32-bytes-long"

type roleLookup string

func (r roleLookup) RoleForUser(_ context.Context, _, _ uuid.UUID) (string, error) {
	return string(r), nil
}

func TestTokenServiceIssueVerifyAndExpiry(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	service, err := NewTokenService(authTestSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	userID, organizationID := uuid.New(), uuid.New()
	token, expires, err := service.Issue(userID, organizationID)
	if err != nil {
		t.Fatal(err)
	}
	if !expires.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected expiry %v", expires)
	}
	principal, err := service.Verify(token)
	if err != nil || principal.UserID != userID || principal.OrganizationID != organizationID {
		t.Fatalf("unexpected verified principal %+v, err=%v", principal, err)
	}
	service.now = func() time.Time { return expires }
	if _, err := service.Verify(token); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestTokenServiceRejectsTampering(t *testing.T) {
	service, _ := NewTokenService(authTestSecret, time.Minute)
	token, _, _ := service.Issue(uuid.New(), uuid.New())
	if _, err := service.Verify(token + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err := service.Verify("e30.e30.AA"); err == nil {
		t.Fatal("unsupported or unsigned token accepted")
	}
}

func TestBearerLoadsCurrentRoleAndEnforcesAuthorization(t *testing.T) {
	service, _ := NewTokenService(authTestSecret, time.Minute)
	userID, orgID := uuid.New(), uuid.New()
	token, _, _ := service.Issue(userID, orgID)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFrom(r.Context())
		if !ok || principal.Role != RoleViewer || principal.UserID != userID || principal.OrganizationID != orgID {
			t.Errorf("unexpected principal: %+v", principal)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Bearer(service, roleLookup(RoleViewer))(RequireRoles(RoleAdmin, RoleViewer)(next))
	req := httptest.NewRequest(http.MethodGet, "/v1/channels", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("viewer should pass read policy, got %d", res.Code)
	}

	forbidden := Bearer(service, roleLookup(RoleViewer))(RequireRoles(RoleAdmin)(next))
	res = httptest.NewRecorder()
	forbidden.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("viewer should be denied admin operation, got %d", res.Code)
	}
}
