package asset

import (
	"encoding/json"
	"net/url"
	"strings"
)

// RedactURL removes query strings and fragments, which may contain signed
// object-store credentials or playback tokens, while retaining useful origin
// context for clients and diagnostics.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		if raw == "" {
			return ""
		}
		return "[redacted]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// RedactText sanitizes URLs embedded in errors before they reach HTTP responses,
// metadata or logs. It intentionally handles the common URL delimiters emitted
// by net/http errors without attempting to parse arbitrary prose.
func RedactText(text string) string {
	fields := strings.Fields(text)
	for i, field := range fields {
		trimmed := strings.Trim(field, "()[]{}<>,;\"'")
		if u, err := url.Parse(trimmed); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
			fields[i] = strings.Replace(field, trimmed, RedactURL(trimmed), 1)
		}
	}
	return strings.Join(fields, " ")
}

func (a Asset) MarshalJSON() ([]byte, error) {
	type response Asset
	return json.Marshal(struct {
		response
		HLSURL string `json:"hls_url"`
	}{
		response: response(a),
		HLSURL:   RedactURL(a.HLSURL),
	})
}
