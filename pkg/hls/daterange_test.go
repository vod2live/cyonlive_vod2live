package hls

import (
	"strings"
	"testing"
)

func TestDateRangeRoundTrip(t *testing.T) {
	input := "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXT-X-DATERANGE:ID=\"ad-1\",CLASS=\"com.example.ad\",DURATION=30\n#EXTINF:6,\nsegment.m4s\n"
	parsed, err := ParseMedia(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.DateRanges) != 1 {
		t.Fatalf("got %d date ranges", len(parsed.DateRanges))
	}
	rendered := RenderMedia(parsed, 1, false)
	if !strings.Contains(rendered, "#EXT-X-DATERANGE:ID=\"ad-1\"") {
		t.Fatal("daterange was not rendered")
	}
}
