package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIKey(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := APIKey("secret")(next)
	for _, tc := range []struct {
		name, key string
		status    int
	}{{"missing", "", 401}, {"wrong", "wrong", 401}, {"valid", "secret", 204}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1", nil)
			if tc.key != "" {
				req.Header.Set("X-API-Key", tc.key)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.status {
				t.Fatalf("got %d want %d", rr.Code, tc.status)
			}
		})
	}
}
