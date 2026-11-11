package hls

import "testing"

func TestParseMasterAndRender(t *testing.T) {
	p, err := ParseMaster("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=1280x720,CODECS=\"avc1\"\n720/index.m3u8\n")
	if err != nil || len(p.Variants) != 1 || p.Variants[0].Height != 720 {
		t.Fatalf("unexpected master: %#v, %v", p, err)
	}
	out := RenderMaster(p, "/v1/channels/test/media")
	if out == "" || !contains(out, "720p.m3u8") {
		t.Fatalf("unexpected rendered master: %s", out)
	}
}

func TestParseMedia(t *testing.T) {
	p, err := ParseMedia("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6.000,\nseg-1.m4s\n#EXTINF:4.000,\nseg-2.m4s\n")
	if err != nil || len(p.Segments) != 2 || p.Segments[1].Duration != 4 {
		t.Fatalf("unexpected media: %#v, %v", p, err)
	}
}

func TestParseAndRenderDiscontinuitySequence(t *testing.T) {
	input := "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:10\n#EXT-X-DISCONTINUITY-SEQUENCE:4\n#EXTINF:6,\na.ts\n#EXT-X-DISCONTINUITY\n#EXTINF:6,\nb.ts\n"
	p, err := ParseMedia(input)
	if err != nil || p.DiscontinuitySequence != 4 || !p.Segments[1].Discontinuity {
		t.Fatalf("unexpected discontinuity state: %#v, %v", p, err)
	}
	out := RenderMedia(p, p.MediaSequence, false)
	if !contains(out, "#EXT-X-DISCONTINUITY-SEQUENCE:4") || !contains(out, "#EXT-X-DISCONTINUITY\n#EXTINF:6.000,\nb.ts") {
		t.Fatalf("discontinuity metadata was not rendered: %s", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
