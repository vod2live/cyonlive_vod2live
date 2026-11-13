package config

import "os"

type MediaConfig struct {
	StorageEndpoint  string
	StorageAccessKey string
	StorageSecretKey string
	StorageBucket    string
	StorageUseSSL    bool
	StoragePublicURL string
	MediaTempDir     string
	FFmpegPath       string
	MediaPollSeconds int
	KeepOriginal     bool
	MetricsAddr      string
	AbandonedUploadTTLHours int
	CleanupIntervalSeconds int
}

func LoadMedia() MediaConfig {
	return MediaConfig{
		StorageEndpoint:  env("MINIO_ENDPOINT", "localhost:9000"),
		StorageAccessKey: env("MINIO_ACCESS_KEY", "minioadmin"),
		StorageSecretKey: env("MINIO_SECRET_KEY", "minioadmin"),
		StorageBucket:    env("MINIO_BUCKET", "vod2live"),
		StorageUseSSL:    boolEnv("MINIO_USE_SSL", false),
		StoragePublicURL: env("MINIO_PUBLIC_URL", "http://localhost:9000"),
		MediaTempDir:     env("MEDIA_TEMP_DIR", "/tmp/vod2live-media"),
		FFmpegPath:       env("FFMPEG_PATH", "ffmpeg"),
		MediaPollSeconds: intEnv("MEDIA_POLL_SECONDS", 5),
		KeepOriginal:     boolEnv("MEDIA_KEEP_ORIGINAL", false),
		MetricsAddr:      env("MEDIA_METRICS_ADDR", ":9091"),
		AbandonedUploadTTLHours: intEnv("MEDIA_ABANDONED_UPLOAD_TTL_HOURS", 24),
		CleanupIntervalSeconds: intEnv("MEDIA_CLEANUP_INTERVAL_SECONDS", 900),
	}
}

func storageEnvPresent() bool {
	return os.Getenv("MINIO_ENDPOINT") != ""
}
