package asset

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInspectorReadsMasterDurationFromMediaVariant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		switch r.URL.Path {
		case "/master.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=640x360\nlow/index.m3u8\n"))
		case "/low/index.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\na.m4s\n#EXTINF:4,\nb.m4s\n#EXT-X-ENDLIST\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	got, err := (HLSInspector{AllowPrivate: true}).Inspect(server.URL + "/master.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsMaster || got.Variants != 1 || got.Segments != 2 || got.DurationMS != 10000 {
		t.Fatalf("unexpected inspection: %+v", got)
	}
}

func TestInspectorBlocksPrivateOriginByDefault(t *testing.T) {
	if _, err := (HLSInspector{}).Inspect("http://127.0.0.1/master.m3u8"); err == nil {
		t.Fatal("expected private origin to be blocked")
	}
}
