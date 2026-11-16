package metrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	if !strings.Contains(rr.Body.String(), `vod2live_http_requests_by_status_total{status="204"} 1`) {
		t.Fatalf("missing status metric: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "vod2live_http_request_duration_seconds_count 1") {
		t.Fatalf("missing latency metric: %s", rr.Body.String())
	}
}

func TestMediaMetrics(t *testing.T) {
	m := &Metrics{}
	m.SetMediaWorkerState("processing")
	m.ObserveMediaJob("completed", 6*time.Second)
	m.ObserveMediaJob("failed", 2*time.Second)
	m.ObserveMediaJob("ignored", time.Second)

	rr := httptest.NewRecorder()
	m.Handler(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rr.Body.String()
	for _, metric := range []string{
		`vod2live_media_jobs_total{status="completed"} 1`,
		`vod2live_media_jobs_total{status="failed"} 1`,
		`vod2live_media_job_duration_seconds_count{status="completed"} 1`,
		`vod2live_media_job_duration_seconds_count{status="failed"} 1`,
		`vod2live_media_worker_state{state="processing"} 1`,
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("missing media metric %q in %s", metric, body)
		}
	}
	if strings.Contains(body, `status="ignored"`) {
		t.Fatalf("unexpected unsupported status in metrics: %s", body)
	}
}

func TestPlayoutMetricsDistinguishSkippedLease(t *testing.T) {
	m := &Metrics{}
	m.ObservePlayoutTick(nil)
	m.ObservePlayoutTick(errors.New("worker failed"))
	m.ObservePlayoutSkipped()

	rr := httptest.NewRecorder()
	m.Handler(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rr.Body.String()
	for _, metric := range []string{
		"vod2live_playout_ticks_total 2",
		"vod2live_playout_errors_total 1",
		"vod2live_playout_skipped_total 1",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("missing playout metric %q in %s", metric, body)
		}
	}
}

func TestProtectedMetricsRequiresAndAcceptsAPIKey(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := Protected("metrics-secret", next)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	authorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer metrics-secret")
	handler.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d, want %d", authorized.Code, http.StatusNoContent)
	}
}
