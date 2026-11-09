package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	RedisAddr       string
	APIKey          string
	AllowPrivateHLS bool
}

func Load() Config {
	return Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", "postgres://vod2live:vod2live@localhost:5432/vod2live?sslmode=disable"),
		RedisAddr:       env("REDIS_ADDR", "localhost:6379"),
		APIKey:          env("API_KEY", ""),
		AllowPrivateHLS: boolEnv("ALLOW_PRIVATE_HLS", false),
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
