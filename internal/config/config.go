package config

import "os"

type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	RedisAddr    string
}

func Load() Config {
	return Config{
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://vod2live:vod2live@localhost:5432/vod2live?sslmode=disable"),
		RedisAddr:   env("REDIS_ADDR", "localhost:6379"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" { return value }
	return fallback
}
