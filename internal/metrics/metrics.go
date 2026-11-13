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
var mediaLatencyBuckets = []float64{1, 5, 15, 30, 60, 300, 900}

type Metrics struct {
	Requests         atomic.Uint64
	Errors           atomic.Uint64
	ManifestRequests atomic.Uint64
	mu               sync.Mutex
	byStatus         map[int]uint64
	latencyCount     uint64
	latencySum       float64
	latencyBuckets   []uint64
	mediaJobs        map[string]uint64
	mediaDurations   map[string]durationSummary
	mediaWorkerState string
}

type durationSummary struct {
	count   uint64
	sum     float64
	buckets []uint64
}

// ObserveMediaJob records one terminal media-job outcome and its processing
// duration in seconds. Status values are intentionally limited to completed
// and failed to keep Prometheus cardinality bounded.
func (m *Metrics) ObserveMediaJob(status string, duration time.Duration) {
	if status != "completed" && status != "failed" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mediaJobs == nil {
		m.mediaJobs = make(map[string]uint64)
	}
	if m.mediaDurations == nil {
		m.mediaDurations = make(map[string]durationSummary)
	}
	m.mediaJobs[status]++
	summary := m.mediaDurations[status]
	if summary.buckets == nil {
		summary.buckets = make([]uint64, len(mediaLatencyBuckets))
	}
	seconds := duration.Seconds()
	summary.count++
	summary.sum += seconds
	for i, bound := range mediaLatencyBuckets {
		if seconds <= bound {
			summary.buckets[i]++
		}
	}
	m.mediaDurations[status] = summary
}

func (m *Metrics) SetMediaWorkerState(state string) {
	switch state {
	case "starting", "idle", "processing", "stopping", "failed":
	default:
		return
	}
	m.mu.Lock()
	m.mediaWorkerState = state
	m.mu.Unlock()
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
	mediaJobs := make(map[string]uint64, len(m.mediaJobs))
	for status, value := range m.mediaJobs {
		mediaJobs[status] = value
	}
	mediaDurations := make(map[string]durationSummary, len(m.mediaDurations))
	for status, summary := range m.mediaDurations {
		summary.buckets = append([]uint64(nil), summary.buckets...)
		mediaDurations[status] = summary
	}
	mediaWorkerState := m.mediaWorkerState
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
	for _, status := range []string{"completed", "failed"} {
		fmt.Fprintf(w, "vod2live_media_jobs_total{status=%q} %d\n", status, mediaJobs[status])
		summary := mediaDurations[status]
		for i, bound := range mediaLatencyBuckets {
			value := uint64(0)
			if i < len(summary.buckets) {
				value = summary.buckets[i]
			}
			fmt.Fprintf(w, "vod2live_media_job_duration_seconds_bucket{status=%q,le=%q} %d\n", status, strconv.FormatFloat(bound, 'f', -1, 64), value)
		}
		fmt.Fprintf(w, "vod2live_media_job_duration_seconds_bucket{status=%q,le=\"+Inf\"} %d\n", status, summary.count)
		fmt.Fprintf(w, "vod2live_media_job_duration_seconds_sum{status=%q} %f\n", status, summary.sum)
		fmt.Fprintf(w, "vod2live_media_job_duration_seconds_count{status=%q} %d\n", status, summary.count)
	}
	if mediaWorkerState != "" {
		fmt.Fprintf(w, "vod2live_media_worker_state{state=%q} 1\n", mediaWorkerState)
	}
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
