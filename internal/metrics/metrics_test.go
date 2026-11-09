package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsMiddlewareAndHandler(t *testing.T) {
	m := &Metrics{}
	r := http.NewServeMux()
	r.Handle("/hello", m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))
	rr := httptest.NewRecorder()
	m.Handler(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rr.Body.String(), "vod2live_http_requests_total 1") {
		t.Fatalf("unexpected metrics: %s", rr.Body.String())
	}
}
