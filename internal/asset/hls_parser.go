package asset

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HLSInspector struct{ Client *http.Client }

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
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || !strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
		return Inspection{}, errors.New("hls_url deve ser uma URL http(s) apontando para um manifesto .m3u8")
	}
	client := i.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return Inspection{}, fmt.Errorf("baixar manifesto: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Inspection{}, fmt.Errorf("manifesto retornou HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Inspection{}, err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U") {
		return Inspection{}, errors.New("manifesto não começa com #EXTM3U")
	}
	result := Inspection{URL: rawURL, ContentType: resp.Header.Get("Content-Type"), Bytes: int64(len(body))}
	result.IsMaster = strings.Contains(string(body), "#EXT-X-STREAM-INF")
	s := bufio.NewScanner(strings.NewReader(string(body)))
	var duration float64
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			result.Variants++
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			value := strings.TrimPrefix(line, "#EXTINF:")
			value = strings.SplitN(value, ",", 2)[0]
			if d, e := strconv.ParseFloat(value, 64); e == nil {
				duration += d
				result.Segments++
			}
		}
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			result.TargetDuration, _ = strconv.ParseFloat(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"), 64)
		}
	}
	result.DurationMS = int64(duration * 1000)
	if !result.IsMaster && result.Segments == 0 {
		return Inspection{}, errors.New("media playlist sem segmentos EXTINF")
	}
	if result.IsMaster && result.Variants == 0 {
		return Inspection{}, errors.New("master playlist sem variantes")
	}
	return result, nil
}
