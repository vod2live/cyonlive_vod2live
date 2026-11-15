package asset

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRedactURLRemovesSignedQueryAndCredentials(t *testing.T) {
	raw := "https://user:pass@example.test/live/master.m3u8?X-Amz-Credential=secret&token=secret#fragment"
	got := RedactURL(raw)
	if got != "https://example.test/live/master.m3u8" {
		t.Fatalf("RedactURL() = %q", got)
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "pass") {
		t.Fatalf("redacted URL still contains secret material: %q", got)
	}
}

func TestAssetJSONRedactsHLSURL(t *testing.T) {
	item := Asset{ID: uuid.New(), HLSURL: "https://cdn.test/master.m3u8?token=do-not-leak"}
	body, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "do-not-leak") || !strings.Contains(text, `"hls_url":"https://cdn.test/master.m3u8"`) {
		t.Fatalf("asset JSON did not redact hls_url: %s", text)
	}
}
