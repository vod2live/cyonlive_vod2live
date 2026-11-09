package asset

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func validateOrigin(ctx context.Context, rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.User != nil {
		return fmt.Errorf("HLS URL não pode conter credenciais embutidas")
	}
	if allowPrivate {
		return nil
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("HLS URL sem host")
	}
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("origem HLS local bloqueada por proteção SSRF")
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolver origem HLS: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("origem HLS em rede privada bloqueada: %s", host)
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
