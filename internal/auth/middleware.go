package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func APIKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				next.ServeHTTP(w, r)
				return
			}
			provided := r.Header.Get("X-API-Key")
			if provided == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(auth, "Bearer ") {
					provided = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
				}
			}
			if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="vod2live"`)
				http.Error(w, "não autorizado", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
