package metrics

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Protected wraps a metrics handler. Development may leave apiKey empty; when
// configured, both the explicit X-API-Key header and a Bearer token are
// accepted so Prometheus or a reverse proxy can authenticate the scrape.
func Protected(apiKey string, next http.Handler) http.Handler {
	if apiKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if provided == "" {
			authorization := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(authorization, "Bearer ") {
				provided = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
			}
		}
		if len(provided) != len(apiKey) || subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "não autorizado", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
