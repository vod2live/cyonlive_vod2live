package config

import "fmt"

func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("HTTP_ADDR não pode ser vazio")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL não pode ser vazio")
	}
	if c.RedisAddr == "" {
		return fmt.Errorf("REDIS_ADDR não pode ser vazio")
	}
	if c.HLSMaxManifestBytes <= 0 {
		return fmt.Errorf("HLS_MAX_MANIFEST_BYTES deve ser positivo")
	}
	if c.HLSRequestTimeoutSecs <= 0 {
		return fmt.Errorf("HLS_REQUEST_TIMEOUT_SECONDS deve ser positivo")
	}
	if c.RateLimitPerMinute <= 0 {
		return fmt.Errorf("RATE_LIMIT_PER_MINUTE deve ser positivo")
	}
	if c.Environment == "production" {
		if c.APIKey == "" {
			return fmt.Errorf("API_KEY é obrigatório em production")
		}
		if c.AllowPrivateHLS {
			return fmt.Errorf("ALLOW_PRIVATE_HLS deve ser false em production")
		}
	}
	return nil
}
