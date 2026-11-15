package manifest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vod2live/cyonlive_vod2live/internal/safefetch"
)

func TestFetchBlocksPrivateHTTPSourceByDefault(t *testing.T) {
	service := Service{}
	if _, err := service.fetch(context.Background(), "http://127.0.0.1/internal.m3u8"); err == nil {
		t.Fatal("expected private upstream to be blocked")
	}
}

func TestFetchLimitsChunkedManifestBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(strings.Repeat("x", 128)))
	}))
	defer server.Close()

	service := Service{
		Client:           safefetch.NewClient(time.Second, true),
		AllowPrivate:     true,
		MaxManifestBytes: 32,
	}
	if _, err := service.fetch(context.Background(), server.URL+"/manifest.m3u8"); err == nil {
		t.Fatal("expected oversized manifest response to be rejected")
	}
}
