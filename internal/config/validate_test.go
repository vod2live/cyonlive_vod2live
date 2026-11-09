package config

import "testing"

func TestProductionRequiresAPIKey(t *testing.T) {
	c := Config{Environment: "production", HTTPAddr: ":8080", DatabaseURL: "postgres://x", RedisAddr: "redis:6379", HLSMaxManifestBytes: 1, HLSRequestTimeoutSecs: 1, RateLimitPerMinute: 1, PlaybackTokenTTLSeconds: 600, AuthTokenTTLSeconds: 900}
	if err := c.Validate(); err == nil {
		t.Fatal("expected API key requirement")
	}
}
func TestDevelopmentAllowsEmptyAPIKey(t *testing.T) {
	c := Config{Environment: "development", HTTPAddr: ":8080", DatabaseURL: "postgres://x", RedisAddr: "redis:6379", HLSMaxManifestBytes: 1, HLSRequestTimeoutSecs: 1, RateLimitPerMinute: 1, PlaybackTokenTTLSeconds: 600, AuthTokenTTLSeconds: 900}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProductionRejectsDevelopmentPlaybackSecret(t *testing.T) {
	c := Config{
		Environment: "production", HTTPAddr: ":8080", DatabaseURL: "postgres://x", RedisAddr: "redis:6379",
		APIKey: "admin-key", PlaybackTokenSecret: "development-only-playback-secret-change-me",
		PlaybackTokenTTLSeconds: 600, AuthTokenSecret: "test-auth-secret-at-least-32-bytes-long", AuthTokenTTLSeconds: 900,
		HLSMaxManifestBytes: 1, HLSRequestTimeoutSecs: 1, RateLimitPerMinute: 1,
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected development playback secret to be rejected")
	}
}

func TestProductionRequiresIndependentAuthSecret(t *testing.T) {
	c := Config{
		Environment: "production", HTTPAddr: ":8080", DatabaseURL: "postgres://x", RedisAddr: "redis:6379",
		APIKey: "admin-key", PlaybackTokenSecret: "random-playback-secret-at-least-32-bytes",
		AuthTokenSecret: "random-playback-secret-at-least-32-bytes", PlaybackTokenTTLSeconds: 600, AuthTokenTTLSeconds: 900,
		HLSMaxManifestBytes: 1, HLSRequestTimeoutSecs: 1, RateLimitPerMinute: 1,
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected auth and playback secrets to be independent")
	}
}
