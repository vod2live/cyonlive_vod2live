package playback

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManifestETag(t *testing.T) {
	h := Handler{CORSOrigin: "https://player.example"}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.writeManifest(rr, r, []byte("#EXTM3U\n"))
	etag := rr.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing etag")
	}
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.Header.Set("If-None-Match", etag)
	rr2 := httptest.NewRecorder()
	h.writeManifest(rr2, r2, []byte("#EXTM3U\n"))
	if rr2.Code != http.StatusNotModified {
		t.Fatalf("got %d", rr2.Code)
	}
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.Header.Set("Origin", "https://player.example")
	rr3 := httptest.NewRecorder()
	h.writeManifest(rr3, r3, []byte(strings.Repeat("x", 1)))
	if rr3.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("missing cors")
	}
}
