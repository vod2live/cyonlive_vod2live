package hls

import (
	"bufio"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func ParseMaster(text string) (MasterPlaylist, error) {
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return MasterPlaylist{}, fmt.Errorf("invalid HLS header")
	}
	var out MasterPlaylist
	lines := lines(text)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") && i+1 < len(lines) {
			v := parseAttributes(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
			variant := Variant{URI: lines[i+1], Bandwidth: int64(parseInt(v["BANDWIDTH"])), Width: parseResolution(v["RESOLUTION"]).width, Height: parseResolution(v["RESOLUTION"]).height, Codecs: v["CODECS"], AudioGroup: v["AUDIO"]}
			out.Variants = append(out.Variants, variant)
			i++
		}
		if strings.HasPrefix(line, "#EXT-X-MEDIA:") {
			v := parseAttributes(strings.TrimPrefix(line, "#EXT-X-MEDIA:"))
			r := Rendition{URI: v["URI"], GroupID: v["GROUP-ID"], Name: v["NAME"], Language: v["LANGUAGE"], Default: v["DEFAULT"] == "YES"}
			if v["TYPE"] == "AUDIO" {
				out.Audio = append(out.Audio, r)
			}
			if v["TYPE"] == "SUBTITLES" {
				out.Subtitles = append(out.Subtitles, r)
			}
		}
	}
	if len(out.Variants) == 0 {
		return MasterPlaylist{}, fmt.Errorf("master without variants")
	}
	return out, nil
}

func ParseMedia(text string) (MediaPlaylist, error) {
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return MediaPlaylist{}, fmt.Errorf("invalid HLS header")
	}
	var out MediaPlaylist
	lines := lines(text)
	var pending float64
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			out.TargetDuration = int(parseInt(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:")))
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			out.MediaSequence = uint64(parseInt(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:")))
		case strings.HasPrefix(line, "#EXT-X-DATERANGE:"):
			out.DateRanges = append(out.DateRanges, line)
		case strings.HasPrefix(line, "#EXTINF:"):
			value := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			pending, _ = strconv.ParseFloat(value, 64)
		case line == "#EXT-X-DISCONTINUITY":
			if len(out.Segments) > 0 {
				out.Segments[len(out.Segments)-1].Discontinuity = true
			}
		case line == "#EXT-X-ENDLIST":
			out.EndList = true
		case line != "" && !strings.HasPrefix(line, "#"):
			out.Segments = append(out.Segments, Segment{URI: line, Duration: pending})
			pending = 0
		}
	}
	if len(out.Segments) == 0 {
		return MediaPlaylist{}, fmt.Errorf("media without segments")
	}
	return out, nil
}

func Resolve(base, ref string) string {
	b, _ := url.Parse(base)
	r, _ := url.Parse(ref)
	return b.ResolveReference(r).String()
}
func Lines(text string) []string { return lines(text) }
func lines(text string) []string {
	s := bufio.NewScanner(strings.NewReader(text))
	var out []string
	for s.Scan() {
		out = append(out, strings.TrimSpace(s.Text()))
	}
	return out
}
func parseInt(v string) int { n, _ := strconv.Atoi(v); return n }
func parseResolution(v string) struct{ width, height int } {
	p := strings.Split(v, "x")
	if len(p) == 2 {
		return struct{ width, height int }{parseInt(p[0]), parseInt(p[1])}
	}
	return struct{ width, height int }{}
}
func parseAttributes(s string) map[string]string {
	out := map[string]string{}
	var key, val strings.Builder
	inQuote := false
	readingKey := true
	for _, r := range s + "," {
		if r == '"' {
			inQuote = !inQuote
			val.WriteRune(r)
			continue
		}
		if r == '=' && readingKey {
			readingKey = false
			continue
		}
		if r == ',' && !inQuote {
			k := strings.TrimSpace(key.String())
			v := strings.Trim(strings.TrimSpace(val.String()), "\"")
			if k != "" {
				out[k] = v
			}
			key.Reset()
			val.Reset()
			readingKey = true
			continue
		}
		if readingKey {
			key.WriteRune(r)
		} else {
			val.WriteRune(r)
		}
	}
	return out
}

func RenderMaster(p MasterPlaylist, variantBase string) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n")
	for _, v := range p.Variants {
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d", v.Bandwidth)
		if v.Width > 0 {
			fmt.Fprintf(&b, ",RESOLUTION=%dx%d", v.Width, v.Height)
		}
		if v.Codecs != "" {
			fmt.Fprintf(&b, ",CODECS=\"%s\"", v.Codecs)
		}
		b.WriteString("\n" + variantBase + "/" + variantID(v) + ".m3u8\n")
	}
	return b.String()
}
func RenderMedia(p MediaPlaylist, mediaSequence uint64, endList bool) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:6\n")
	target := p.TargetDuration
	if target == 0 {
		target = 6
	}
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n", target, mediaSequence)
	for _, dateRange := range p.DateRanges {
		b.WriteString(dateRange + "\n")
	}
	for _, s := range p.Segments {
		if s.Discontinuity {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n%s\n", s.Duration, s.URI)
	}
	if endList {
		b.WriteString("#EXT-X-ENDLIST\n")
	}
	return b.String()
}
func variantID(v Variant) string {
	if v.Height > 0 {
		return strconv.Itoa(v.Height) + "p"
	}
	return strconv.FormatInt(v.Bandwidth, 10)
}
func VariantID(v Variant) string { return variantID(v) }
