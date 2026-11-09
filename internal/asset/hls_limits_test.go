package asset

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInspectorRejectsOversizedManifest(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n" + strings.Repeat("x", 32)))
	}))
	defer s.Close()
	if _, err := (HLSInspector{AllowPrivate: true, MaxManifestBytes: 8}).Inspect(s.URL + "/x.m3u8"); err == nil {
		t.Fatal("expected oversized manifest rejection")
	}
}
