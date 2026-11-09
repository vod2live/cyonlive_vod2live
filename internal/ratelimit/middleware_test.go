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
