package config

import (
	"os"
	"strconv"
)

type Config struct {
	Environment             string
	HTTPAddr                string
	DatabaseURL             string
	RedisAddr               string
	APIKey                  string
	PlaybackTokenSecret     string
	PlaybackTokenTTLSeconds int
	AllowPrivateHLS         bool
	HLSMaxManifestBytes     int64
	HLSRequestTimeoutSecs   int
	RateLimitPerMinute      int
}

func Load() Config {
	environment := env("APP_ENV", "development")
	return Config{
		Environment:             environment,
		HTTPAddr:                env("HTTP_ADDR", ":8080"),
		DatabaseURL:             env("DATABASE_URL", "postgres://vod2live:vod2live@localhost:5432/vod2live?sslmode=disable"),
		RedisAddr:               env("REDIS_ADDR", "localhost:6379"),
		APIKey:                  env("API_KEY", ""),
		PlaybackTokenSecret:     env("PLAYBACK_TOKEN_SECRET", "development-only-playback-secret-change-me"),
		PlaybackTokenTTLSeconds: intEnv("PLAYBACK_TOKEN_TTL_SECONDS", 600),
		AllowPrivateHLS:         boolEnv("ALLOW_PRIVATE_HLS", false),
		HLSMaxManifestBytes:     int64Env("HLS_MAX_MANIFEST_BYTES", 8<<20),
		HLSRequestTimeoutSecs:   intEnv("HLS_REQUEST_TIMEOUT_SECONDS", 15),
		RateLimitPerMinute:      intEnv("RATE_LIMIT_PER_MINUTE", 120),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}
func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
func int64Env(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
