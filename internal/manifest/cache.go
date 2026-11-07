package manifest

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type ManifestCache struct {
	Client *redis.Client
	TTL    time.Duration
}

func (c ManifestCache) Get(ctx context.Context, key string) ([]byte, bool) {
	if c.Client == nil {
		return nil, false
	}
	value, err := c.Client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return value, true
}
func (c ManifestCache) Set(ctx context.Context, key string, value []byte) {
	if c.Client == nil {
		return
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = 2 * time.Second
	}
	_ = c.Client.Set(ctx, key, value, ttl).Err()
}
