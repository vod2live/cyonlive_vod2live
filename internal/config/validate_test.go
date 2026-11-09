package config

import "testing"

func TestProductionRequiresAPIKey(t *testing.T) {
	c := Config{Environment: "production", HTTPAddr: ":8080", DatabaseURL: "postgres://x", RedisAddr: "redis:6379", HLSMaxManifestBytes: 1, HLSRequestTimeoutSecs: 1, RateLimitPerMinute: 1}
	if err := c.Validate(); err == nil {
		t.Fatal("expected API key requirement")
	}
}
func TestDevelopmentAllowsEmptyAPIKey(t *testing.T) {
	c := Config{Environment: "development", HTTPAddr: ":8080", DatabaseURL: "postgres://x", RedisAddr: "redis:6379", HLSMaxManifestBytes: 1, HLSRequestTimeoutSecs: 1, RateLimitPerMinute: 1}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
