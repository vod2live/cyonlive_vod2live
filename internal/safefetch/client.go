package safefetch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.88.99.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/96",
	"::1/128",
	"::ffff:0:0/96",
	"64:ff9b::/96",
	"64:ff9b:1::/48",
	"100::/64",
	"2001:db8::/32",
	"2002::/16",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
)

func mustPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			panic(err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}

// ValidateURL accepts only HTTP(S) URLs without embedded credentials. Unless
// allowPrivate is enabled for a controlled development environment, every DNS
// answer must be globally routable and not in a reserved/special-purpose range.
func ValidateURL(ctx context.Context, u *url.URL, allowPrivate bool) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("upstream URL must be an absolute HTTP(S) URL")
	}
	if u.User != nil {
		return errors.New("upstream URL must not contain embedded credentials")
	}
	if _, err := strconv.Atoi(u.Port()); u.Port() != "" && err != nil {
		return errors.New("upstream URL has an invalid port")
	}
	host := strings.TrimSuffix(strings.TrimSpace(u.Hostname()), ".")
	if host == "" {
		return errors.New("upstream URL has no hostname")
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		if allowPrivate {
			return nil
		}
		return fmt.Errorf("private upstream host is blocked: %s", host)
	}
	if allowPrivate {
		return nil
	}
	ips, err := lookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve upstream host: %w", err)
	}
	if len(ips) == 0 {
		return errors.New("upstream host has no IP addresses")
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("upstream host resolves to a non-public address: %s", host)
		}
	}
	return nil
}

// NewClient creates an HTTP client that validates each destination at redirect
// time and again during DNS resolution at the actual socket dial. The transport
// dials the validated IP directly to prevent a DNS rebinding between checking
// a hostname and connecting to it. Proxy environment variables are deliberately
// ignored because a proxy would bypass this destination check.
func NewClient(timeout time.Duration, allowPrivate bool) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext:         safeDialContext(dialer, allowPrivate),
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("upstream redirect limit exceeded")
			}
			if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New("HTTPS downgrade redirect is blocked")
			}
			if err := ValidateURL(req.Context(), req.URL, allowPrivate); err != nil {
				return fmt.Errorf("redirect destination blocked: %w", err)
			}
			return nil
		},
	}
}

func safeDialContext(dialer *net.Dialer, allowPrivate bool) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if portNumber, parseErr := strconv.Atoi(port); parseErr != nil || portNumber < 1 || portNumber > 65535 {
			return nil, errors.New("invalid upstream port")
		}
		ips, err := lookupHost(ctx, strings.TrimSuffix(host, "."))
		if err != nil {
			return nil, fmt.Errorf("resolve upstream host: %w", err)
		}
		if len(ips) == 0 {
			return nil, errors.New("upstream host has no IP addresses")
		}
		for _, ip := range ips {
			if !allowPrivate && isBlockedIP(ip) {
				return nil, fmt.Errorf("upstream host resolves to a non-public address: %s", host)
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
}

func lookupHost(ctx context.Context, host string) ([]net.IP, error) {
	if literal, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return []net.IP{net.IP(literal.Unmap().AsSlice())}, nil
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(resolved))
	for _, item := range resolved {
		ips = append(ips, item.IP)
	}
	return ips, nil
}

func isBlockedIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
