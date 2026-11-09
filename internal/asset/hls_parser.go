package asset

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vod2live/cyonlive_vod2live/pkg/hls"
)

type HLSInspector struct {
	Client           *http.Client
	AllowPrivate     bool
	MaxManifestBytes int64
	Timeout          time.Duration
}

type Inspection struct {
	URL            string  `json:"url"`
	ContentType    string  `json:"content_type,omitempty"`
	Bytes          int64   `json:"bytes"`
	IsMaster       bool    `json:"is_master"`
	DurationMS     int64   `json:"duration_ms"`
	Variants       int     `json:"variants"`
	Segments       int     `json:"segments"`
	TargetDuration float64 `json:"target_duration,omitempty"`
}

func (i HLSInspector) Inspect(rawURL string) (Inspection, error) {
	maxBytes := i.MaxManifestBytes
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	client := i.Client
	if client == nil {
		timeout := i.Timeout
		if timeout <= 0 {
			timeout = 15 * time.Second
		}
		client = &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("limite de redirects HLS excedido")
			}
			return nil
		}}
	}
	return inspectManifest(client, rawURL, 0, i.AllowPrivate, maxBytes)
}

func inspectManifest(client *http.Client, rawURL string, depth int, allowPrivate bool, maxBytes int64) (Inspection, error) {
	if depth > 2 {
		return Inspection{}, errors.New("HLS manifest nesting exceeds supported depth")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || !strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
		return Inspection{}, errors.New("hls_url deve ser uma URL http(s) apontando para um manifesto .m3u8")
	}
	if err := validateOrigin(context.Background(), rawURL, allowPrivate); err != nil {
		return Inspection{}, err
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return Inspection{}, fmt.Errorf("baixar manifesto: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Inspection{}, fmt.Errorf("manifesto retornou HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return Inspection{}, fmt.Errorf("manifesto excede o limite de %d bytes", maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return Inspection{}, err
	}
	if int64(len(body)) > maxBytes {
		return Inspection{}, fmt.Errorf("manifesto excede o limite de %d bytes", maxBytes)
	}
	text := string(body)
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return Inspection{}, errors.New("manifesto não começa com #EXTM3U")
	}
	result := Inspection{URL: rawURL, ContentType: resp.Header.Get("Content-Type"), Bytes: int64(len(body))}
	result.IsMaster = strings.Contains(text, "#EXT-X-STREAM-INF")
	if result.IsMaster {
		master, err := hls.ParseMaster(text)
		if err != nil {
			return Inspection{}, err
		}
		result.Variants = len(master.Variants)
		if len(master.Variants) == 0 {
			return Inspection{}, errors.New("master playlist sem variantes")
		}
		for _, variant := range master.Variants {
			child, childErr := inspectManifest(client, hls.Resolve(rawURL, variant.URI), depth+1, allowPrivate, maxBytes)
			if childErr != nil {
				return Inspection{}, fmt.Errorf("variante HLS inválida: %w", childErr)
			}
			if child.DurationMS > result.DurationMS {
				result.DurationMS = child.DurationMS
			}
			result.Segments += child.Segments
			if child.TargetDuration > result.TargetDuration {
				result.TargetDuration = child.TargetDuration
			}
		}
		return result, nil
	}
	media, err := hls.ParseMedia(text)
	if err != nil {
		return Inspection{}, err
	}
	result.TargetDuration = float64(media.TargetDuration)
	for _, segment := range media.Segments {
		result.DurationMS += int64(segment.Duration * 1000)
		result.Segments++
	}
	if result.Segments == 0 {
		return Inspection{}, errors.New("media playlist sem segmentos EXTINF")
	}
	return result, nil
}

func scanManifest(text string) (duration float64, segments int, target float64) {
	s := bufio.NewScanner(strings.NewReader(text))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "#EXTINF:") {
			value := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if d, e := strconv.ParseFloat(value, 64); e == nil {
				duration += d
				segments++
			}
		}
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			target, _ = strconv.ParseFloat(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"), 64)
		}
	}
	return
}
