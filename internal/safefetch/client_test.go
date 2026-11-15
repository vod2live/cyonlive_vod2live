package safefetch

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestValidateURLBlocksNonPublicAddresses(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/master.m3u8",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/master.m3u8",
		"http://100.64.0.1/master.m3u8",
		"http://[::1]/master.m3u8",
		"http://[::ffff:7f00:1]/master.m3u8",
		"http://[64:ff9b::7f00:1]/master.m3u8",
		"http://localhost/master.m3u8",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateURL(context.Background(), u, false); err == nil {
				t.Fatalf("ValidateURL(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestValidateURLAllowsPrivateOnlyWhenExplicitlyEnabled(t *testing.T) {
	u, err := url.Parse("http://127.0.0.1/master.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateURL(context.Background(), u, true); err != nil {
		t.Fatalf("private development URL should be allowed when enabled: %v", err)
	}
}

func TestValidateURLRejectsCredentialsAndUnsupportedSchemes(t *testing.T) {
	for _, raw := range []string{"http://user:password@example.com/master.m3u8", "file:///etc/passwd", "ftp://example.com/master.m3u8"} {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateURL(context.Background(), u, true); err == nil {
				t.Fatalf("ValidateURL(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestClientRejectsRedirectToPrivateTarget(t *testing.T) {
	client := NewClient(0, false)
	target, _ := url.Parse("http://127.0.0.1/private")
	from, _ := url.Parse("https://public.example/master.m3u8")
	req := &http.Request{URL: target, Header: make(http.Header)}
	if err := client.CheckRedirect(req, []*http.Request{{URL: from}}); err == nil {
		t.Fatal("expected redirect to private IP to be blocked")
	}
}

func TestClientRejectsHTTPSDowngradeRedirect(t *testing.T) {
	client := NewClient(0, true)
	from, _ := url.Parse("https://public.example/master.m3u8")
	target, _ := url.Parse("http://public.example/master.m3u8")
	req := &http.Request{URL: target, Header: make(http.Header)}
	if err := client.CheckRedirect(req, []*http.Request{{URL: from}}); err == nil {
		t.Fatal("expected HTTPS downgrade redirect to be blocked")
	}
}
