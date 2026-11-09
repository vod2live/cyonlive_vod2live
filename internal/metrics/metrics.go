package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/vod2live/cyonlive_vod2live/internal/auth"
)

type Metrics struct {
	Requests         atomic.Uint64
	Errors           atomic.Uint64
	ManifestRequests atomic.Uint64
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
		_ = started
	})
}

func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "vod2live_http_requests_total %d\n", m.Requests.Load())
	fmt.Fprintf(w, "vod2live_http_errors_total %d\n", m.Errors.Load())
	fmt.Fprintf(w, "vod2live_auth_failures_total %d\n", auth.AuthFailures())
	fmt.Fprintf(w, "vod2live_manifest_requests_total %d\n", m.ManifestRequests.Load())
	fmt.Fprintf(w, "vod2live_ready 1\n")
	fmt.Fprintln(w, "vod2live_last_status_code "+strconv.Itoa(http.StatusOK))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
