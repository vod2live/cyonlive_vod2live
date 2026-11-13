package metrics

import (
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
