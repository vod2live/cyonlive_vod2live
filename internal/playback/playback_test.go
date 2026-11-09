package playback

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const testSecret = "test-only-secret-with-at-least-32-bytes"

func TestNewSignerRequiresLongSecretAndPositiveTTL(t *testing.T) {
	if _, err := NewSigner("too-short", time.Minute); err == nil {
		t.Fatal("short secret accepted")
	}
	if _, err := NewSigner(testSecret, 0); err == nil {
		t.Fatal("zero TTL accepted")
	}
}

type fakeManifests struct {
	master []byte
	media  []byte
}

func (f fakeManifests) Master(context.Context, uuid.UUID) ([]byte, error) { return f.master, nil }
func (f fakeManifests) Media(context.Context, uuid.UUID, string, time.Time) ([]byte, error) {
	return f.media, nil
}

func TestSignerBindsTokenToChannelAndResourceAndExpires(t *testing.T) {
	signer, err := NewSigner(testSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	channelID := uuid.New()
	expires, token := signer.NewToken(channelID, "master", now)
	if _, err := signer.Verify(channelID, "master", formatInt(expires), token, now); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if _, err := signer.Verify(uuid.New(), "master", formatInt(expires), token, now); err == nil {
		t.Fatal("token accepted for a different channel")
	}
	if _, err := signer.Verify(channelID, "media:720p", formatInt(expires), token, now); err == nil {
		t.Fatal("token accepted for a different resource")
	}
	if _, err := signer.Verify(channelID, "master", formatInt(expires), token, now.Add(time.Minute)); err != ErrExpired {
		t.Fatalf("expected expired token, got %v", err)
	}
}

func TestSignerRejectsMalformedAndExcessiveExpiry(t *testing.T) {
	signer, err := NewSigner(testSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	channelID := uuid.New()
	if _, err := signer.Verify(channelID, "master", "01", "bad", now); err == nil {
		t.Fatal("non-canonical expiration accepted")
	}
	tooLate := now.Add(10 * time.Minute).Unix()
	if _, err := signer.Verify(channelID, "master", formatInt(tooLate), "bad", now); err == nil {
		t.Fatal("expiration beyond maximum lifetime accepted")
	}
}

func TestMasterRewritesVariantToSignedPlaybackURL(t *testing.T) {
	signer, _ := NewSigner(testSecret, time.Minute)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	expires := now.Add(time.Minute).Unix()
	token := signer.sign(id, "master", expires)
	h := Handler{Signer: signer, Manifests: fakeManifests{master: []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\n/v1/channels/" + id.String() + "/media/720p.m3u8\n")}, Now: func() time.Time { return now }}
	req := httptest.NewRequest(http.MethodGet, "/playback/"+id.String()+"/master.m3u8?expires="+formatInt(expires)+"&token="+url.QueryEscape(token), nil)
	res := httptest.NewRecorder()
	h.ServeMaster(res, req, id)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "/playback/"+id.String()+"/media/720p.m3u8?expires=") {
		t.Fatalf("variant URL was not rewritten: %s", res.Body.String())
	}
	if strings.Contains(res.Body.String(), "/v1/channels/") {
		t.Fatalf("administrative URL leaked into public manifest: %s", res.Body.String())
	}
}

func TestMediaRewritesSegmentsAndRejectsExpiredToken(t *testing.T) {
	signer, _ := NewSigner(testSecret, time.Minute)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	expires := now.Add(time.Minute).Unix()
	token := signer.sign(id, "media:720p", expires)
	h := Handler{Signer: signer, Manifests: fakeManifests{media: []byte("#EXTM3U\n#EXTINF:4,\nhttps://cdn.example/segment.ts\n")}, Now: func() time.Time { return now }}
	req := httptest.NewRequest(http.MethodGet, "/playback/"+id.String()+"/media/720p.m3u8?expires="+formatInt(expires)+"&token="+url.QueryEscape(token), nil)
	res := httptest.NewRecorder()
	h.ServeMedia(res, req, id, "720p")
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "https://cdn.example") || !strings.Contains(res.Body.String(), "/playback/"+id.String()+"/segment?u=") {
		t.Fatalf("segment URL was not protected: %s", res.Body.String())
	}

	h.Now = func() time.Time { return now.Add(time.Minute) }
	expired := httptest.NewRecorder()
	h.ServeMedia(expired, req, id, "720p")
	if expired.Code != http.StatusGone {
		t.Fatalf("expected 410 for expired token; got %d", expired.Code)
	}
}

func TestSegmentProxyRequiresSignatureAndStreamsContent(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte("segment-data"))
	}))
	defer origin.Close()
	signer, _ := NewSigner(testSecret, time.Minute)
	now := time.Now().UTC()
	id := uuid.New()
	expires := now.Add(time.Minute).Unix()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(origin.URL + "/chunk.ts"))
	token := signer.sign(id, segmentResource+encoded, expires)
	h := Handler{Signer: signer, AllowPrivate: true, Now: func() time.Time { return now }}
	req := httptest.NewRequest(http.MethodGet, "/playback/"+id.String()+"/segment?u="+encoded+"&expires="+formatInt(expires)+"&token="+url.QueryEscape(token), nil)
	res := httptest.NewRecorder()
	h.ServeSegment(res, req, id)
	if res.Code != http.StatusOK || res.Body.String() != "segment-data" {
		t.Fatalf("proxy response status=%d body=%q", res.Code, res.Body.String())
	}
	if res.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unexpected cache policy: %q", res.Header().Get("Cache-Control"))
	}
}

func formatInt(n int64) string {
	return strconv.FormatInt(n, 10)
}
