package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/vod2live/cyonlive_vod2live/internal/asset"
	"github.com/vod2live/cyonlive_vod2live/internal/auth"
)

type validationRepoStub struct {
	item       asset.Asset
	updateErr  error
	updateSeen bool
	status     asset.Status
}

func (s *validationRepoStub) FindByIDInOrg(context.Context, uuid.UUID, uuid.UUID) (asset.Asset, error) {
	return s.item, nil
}

func (s *validationRepoStub) UpdateStatusInOrg(_ context.Context, _ uuid.UUID, _ uuid.UUID, status asset.Status, _ int64, _ any) error {
	s.updateSeen = true
	s.status = status
	return s.updateErr
}

type validationAccessStub struct{}

func (validationAccessStub) UserCanAccessAsset(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type validationMembershipStub struct{}

func (validationMembershipStub) RoleForUser(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return auth.RoleAdmin, nil
}

func validationRequest(t *testing.T, id, userID, organizationID uuid.UUID) *http.Request {
	t.Helper()
	tokens, err := auth.NewTokenService("01234567890123456789012345678901", 5*60*1000000000)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := tokens.Issue(userID, organizationID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/assets/"+id.String()+"/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", id.String())
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func TestValidateAssetReturnsServerErrorWhenValidStatusCannotBePersisted(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_HLS", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nsegment.ts\n#EXT-X-ENDLIST\n"))
	}))
	defer server.Close()

	id, userID, organizationID := uuid.New(), uuid.New(), uuid.New()
	repo := &validationRepoStub{item: asset.Asset{ID: id, OrganizationID: organizationID, HLSURL: server.URL + "/master.m3u8?token=secret"}, updateErr: errors.New("database unavailable")}
	wrapped := auth.Bearer(mustTokenService(t), validationMembershipStub{})(validateAsset(repo, validationAccessStub{}))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, validationRequest(t, id, userID, organizationID))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("validation status = %d, want %d; body=%s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	if response.Code == http.StatusOK {
		t.Fatal("validation returned success after persistence failure")
	}
	if !repo.updateSeen || repo.status != asset.StatusValid {
		t.Fatalf("expected failed valid-status persistence, seen=%v status=%q", repo.updateSeen, repo.status)
	}
	if body := response.Body.String(); body != "falha ao persistir o resultado da validação\n" {
		t.Fatalf("unexpected response body: %q", body)
	}
}

func TestValidateAssetReturnsServerErrorWhenInvalidStatusCannotBePersisted(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_HLS", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	id, userID, organizationID := uuid.New(), uuid.New(), uuid.New()
	repo := &validationRepoStub{item: asset.Asset{ID: id, OrganizationID: organizationID, HLSURL: server.URL + "/master.m3u8?token=secret"}, updateErr: errors.New("database unavailable")}
	wrapped := auth.Bearer(mustTokenService(t), validationMembershipStub{})(validateAsset(repo, validationAccessStub{}))
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, validationRequest(t, id, userID, organizationID))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("validation status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if repo.status != asset.StatusInvalid {
		t.Fatalf("expected invalid status persistence attempt, got %q", repo.status)
	}
}

func mustTokenService(t *testing.T) *auth.TokenService {
	t.Helper()
	service, err := auth.NewTokenService("01234567890123456789012345678901", 5*60*1000000000)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
