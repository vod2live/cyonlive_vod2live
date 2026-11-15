package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLimiterBlocksAfterLimit(t *testing.T) {
	l := New(1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := l.Middleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 204 {
		t.Fatal(rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 429 {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestClientIPIgnoresUntrustedForwardedHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.test", nil)
	req.RemoteAddr = "192.0.2.10:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")

	if got, want := clientIP(req), "192.0.2.10"; got != want {
		t.Fatalf("clientIP() = %q, want %q", got, want)
	}
}
