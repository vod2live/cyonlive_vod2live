package playback

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	masterResource  = "master"
	segmentResource = "segment:"
)

type Signer struct {
	secret []byte
	ttl    time.Duration
}

func NewSigner(secret string, ttl time.Duration) (*Signer, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("PLAYBACK_TOKEN_SECRET deve conter ao menos 32 bytes")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("PLAYBACK_TOKEN_TTL_SECONDS deve ser positivo")
	}
	return &Signer{secret: []byte(secret), ttl: ttl}, nil
}

func (s *Signer) TTL() time.Duration { return s.ttl }

func (s *Signer) NewToken(channelID uuid.UUID, resource string, now time.Time) (int64, string) {
	expires := now.UTC().Add(s.ttl).Unix()
	return expires, s.sign(channelID, resource, expires)
}

func (s *Signer) sign(channelID uuid.UUID, resource string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(channelID.String() + "\n" + strconv.FormatInt(expires, 10) + "\n" + resource))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Signer) Verify(channelID uuid.UUID, resource, expiresText, token string, now time.Time) (int64, error) {
	expires, err := strconv.ParseInt(expiresText, 10, 64)
	if err != nil || strconv.FormatInt(expires, 10) != expiresText {
		return 0, fmt.Errorf("invalid expiration")
	}
	current := now.UTC().Unix()
	if expires <= current {
		return expires, ErrExpired
	}
	if expires > now.UTC().Add(s.ttl).Unix()+1 {
		return 0, fmt.Errorf("expiration exceeds maximum lifetime")
	}
	supplied, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(supplied) != sha256.Size {
		return 0, fmt.Errorf("invalid signature")
	}
	expected, _ := base64.RawURLEncoding.DecodeString(s.sign(channelID, resource, expires))
	if !hmac.Equal(supplied, expected) {
		return 0, fmt.Errorf("invalid signature")
	}
	return expires, nil
}

var ErrExpired = fmt.Errorf("playback token expired")

type ManifestSource interface {
	Master(context.Context, uuid.UUID) ([]byte, error)
	Media(context.Context, uuid.UUID, string, time.Time) ([]byte, error)
}

type Handler struct {
	Signer       *Signer
	Manifests    ManifestSource
	AllowPrivate bool
	Now          func() time.Time
	Client       *http.Client
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

func (h Handler) ServeMaster(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	expires, err := h.verify(r, channelID, masterResource)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	body, err := h.Manifests.Master(r.Context(), channelID)
	if err != nil {
		http.Error(w, "manifest indisponível", http.StatusBadGateway)
		return
	}
	lines := strings.Split(string(body), "\n")
	prefix := "/v1/channels/" + channelID.String() + "/media/"
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, ".m3u8") {
			variant := strings.TrimSuffix(strings.TrimPrefix(line, prefix), ".m3u8")
			if variant == "" || strings.Contains(variant, "/") {
				http.Error(w, "manifest inválido", http.StatusBadGateway)
				return
			}
			resource := "media:" + variant
			token := h.Signer.sign(channelID, resource, expires)
			lines[i] = fmt.Sprintf("/playback/%s/media/%s.m3u8?expires=%d&token=%s", channelID, url.PathEscape(variant), expires, url.QueryEscape(token))
		} else if line != "" && !strings.HasPrefix(line, "#") {
			http.Error(w, "manifest contém URI inesperada", http.StatusBadGateway)
			return
		}
	}
	h.writeManifest(w, []byte(strings.Join(lines, "\n")))
}

func (h Handler) ServeMedia(w http.ResponseWriter, r *http.Request, channelID uuid.UUID, variant string) {
	if variant == "" || strings.Contains(variant, "/") || strings.Contains(variant, "\\") {
		http.Error(w, "variante inválida", http.StatusBadRequest)
		return
	}
	resource := "media:" + variant
	expires, err := h.verify(r, channelID, resource)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	body, err := h.Manifests.Media(r.Context(), channelID, variant, h.now())
	if err != nil {
		http.Error(w, "manifest indisponível", http.StatusBadGateway)
		return
	}
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parsed, parseErr := url.Parse(line)
		if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			http.Error(w, "manifest contém URI de segmento inválida", http.StatusBadGateway)
			return
		}
		target := base64.RawURLEncoding.EncodeToString([]byte(line))
		token := h.Signer.sign(channelID, segmentResource+target, expires)
		lines[i] = fmt.Sprintf("/playback/%s/segment?u=%s&expires=%d&token=%s", channelID, target, expires, url.QueryEscape(token))
	}
	h.writeManifest(w, []byte(strings.Join(lines, "\n")))
}

func (h Handler) ServeSegment(w http.ResponseWriter, r *http.Request, channelID uuid.UUID) {
	values := r.URL.Query()
	if len(values["u"]) != 1 || len(values["expires"]) != 1 || len(values["token"]) != 1 {
		http.Error(w, "playback não autorizado", http.StatusForbidden)
		return
	}
	target := values.Get("u")
	if _, err := h.verify(r, channelID, segmentResource+target); err != nil {
		h.writeAuthError(w, err)
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(target)
	if err != nil || len(raw) > 4096 {
		http.Error(w, "segmento inválido", http.StatusBadRequest)
		return
	}
	parsed, err := url.Parse(string(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "segmento inválido", http.StatusBadRequest)
		return
	}
	client := h.Client
	if client == nil {
		client = &http.Client{
			Timeout:       20 * time.Second,
			Transport:     &http.Transport{DialContext: h.dialContext},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		http.Error(w, "segmento inválido", http.StatusBadRequest)
		return
	}
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		upstreamReq.Header.Set("Range", rangeHeader)
	}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		http.Error(w, "segmento indisponível", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "segmento indisponível", http.StatusBadGateway)
		return
	}
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h Handler) verify(r *http.Request, channelID uuid.UUID, resource string) (int64, error) {
	values := r.URL.Query()
	if len(values["expires"]) != 1 || len(values["token"]) != 1 {
		return 0, fmt.Errorf("missing token")
	}
	return h.Signer.Verify(channelID, resource, values.Get("expires"), values.Get("token"), h.now())
}

func (h Handler) writeAuthError(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if err == ErrExpired {
		http.Error(w, "playback expirado", http.StatusGone)
		return
	}
	http.Error(w, "playback não autorizado", http.StatusForbidden)
}

func (h Handler) writeManifest(w http.ResponseWriter, body []byte) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h Handler) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if h.AllowPrivate {
		return (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, address)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("não foi possível resolver origem HLS")
	}
	for _, item := range ips {
		if blockedIP(item.IP) {
			return nil, fmt.Errorf("origem HLS em rede privada bloqueada")
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, item := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(item.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
