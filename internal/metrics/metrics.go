package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vod2live/cyonlive_vod2live/internal/auth"
	"github.com/vod2live/cyonlive_vod2live/internal/buildinfo"
)

var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type Metrics struct {
	Requests         atomic.Uint64
	Errors           atomic.Uint64
	ManifestRequests atomic.Uint64
	mu               sync.Mutex
	byStatus         map[int]uint64
	latencyCount     uint64
	latencySum       float64
	latencyBuckets   []uint64
}

func (m *Metrics) observe(status int, seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byStatus == nil {
		m.byStatus = make(map[int]uint64)
	}
	if m.latencyBuckets == nil {
		m.latencyBuckets = make([]uint64, len(latencyBuckets))
	}
	m.byStatus[status]++
	m.latencyCount++
	m.latencySum += seconds
	for i, bound := range latencyBuckets {
		if seconds <= bound {
			m.latencyBuckets[i]++
		}
	}
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.Requests.Add(1)
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/v1/" && len(r.URL.Path) >= 9 && r.URL.Path[:9] == "/v1/channels" && len(r.URL.Path) >= 8 && r.URL.Path[len(r.URL.Path)-8:] == ".m3u8" {
			m.ManifestRequests.Add(1)
		}
		started := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		if rw.status >= 400 {
			m.Errors.Add(1)
		}
		m.observe(rw.status, time.Since(started).Seconds())
	})
}

func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "vod2live_http_requests_total %d\n", m.Requests.Load())
	fmt.Fprintf(w, "vod2live_http_errors_total %d\n", m.Errors.Load())
	fmt.Fprintf(w, "vod2live_auth_failures_total %d\n", auth.AuthFailures())
	fmt.Fprintf(w, "vod2live_manifest_requests_total %d\n", m.ManifestRequests.Load())
	fmt.Fprintln(w, "vod2live_ready 1")
	fmt.Fprintln(w, "vod2live_last_status_code "+strconv.Itoa(http.StatusOK))
	m.mu.Lock()
	statuses := make(map[int]uint64, len(m.byStatus))
	for status, count := range m.byStatus {
		statuses[status] = count
	}
	buckets := append([]uint64(nil), m.latencyBuckets...)
	count, sum := m.latencyCount, m.latencySum
	m.mu.Unlock()
	keys := make([]int, 0, len(statuses))
	for status := range statuses {
		keys = append(keys, status)
	}
	sort.Ints(keys)
	for _, status := range keys {
		fmt.Fprintf(w, "vod2live_http_requests_by_status_total{status=%q} %d\n", strconv.Itoa(status), statuses[status])
	}
	for i, bound := range latencyBuckets {
		value := uint64(0)
		if i < len(buckets) {
			value = buckets[i]
		}
		fmt.Fprintf(w, "vod2live_http_request_duration_seconds_bucket{le=%q} %d\n", strconv.FormatFloat(bound, 'f', -1, 64), value)
	}
	fmt.Fprintf(w, "vod2live_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", count)
	fmt.Fprintf(w, "vod2live_http_request_duration_seconds_sum %f\n", sum)
	fmt.Fprintf(w, "vod2live_http_request_duration_seconds_count %d\n", count)
	fmt.Fprintf(w, "vod2live_build_info{version=%q,commit=%q} 1\n", buildinfo.Version, buildinfo.Commit)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
