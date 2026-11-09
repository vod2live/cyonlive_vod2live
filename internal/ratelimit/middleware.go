package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]entry
}
type entry struct {
	started time.Time
	count   int
}

func New(limit int) *Limiter {
	if limit <= 0 {
		limit = 120
	}
	return &Limiter{limit: limit, window: time.Minute, clients: map[string]entry{}}
}
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	item := l.clients[key]
	if item.started.IsZero() || now.Sub(item.started) >= l.window {
		l.clients[key] = entry{started: now, count: 1}
		return true
	}
	if item.count >= l.limit {
		return false
	}
	item.count++
	l.clients[key] = item
	return true
}
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(clientIP(r), time.Now()) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit excedido", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func clientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func LimitFromString(value string, fallback int) int {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
