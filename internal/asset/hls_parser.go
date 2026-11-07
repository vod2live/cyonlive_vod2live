package asset

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HLSInspector struct { Client *http.Client }

type Inspection struct {
	URL string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Bytes int64 `json:"bytes"`
	IsMaster bool `json:"is_master"`
	TargetDuration float64 `json:"target_duration,omitempty"`
}

func (i HLSInspector) Inspect(rawURL string) (Inspection, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
		return Inspection{}, errors.New("hls_url inválida: informe uma URL http(s) para um manifesto .m3u8")
	}
	if !strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
		return Inspection{}, errors.New("hls_url deve apontar para um manifesto .m3u8")
	}
	client := i.Client
	if client == nil { client = &http.Client{Timeout: 15 * time.Second} }
	resp, err := client.Get(rawURL)
	if err != nil { return Inspection{}, fmt.Errorf("baixar manifesto: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return Inspection{}, fmt.Errorf("manifesto retornou HTTP %d", resp.StatusCode) }
	return Inspection{URL: rawURL, ContentType: resp.Header.Get("Content-Type"), Bytes: resp.ContentLength}, nil
}
