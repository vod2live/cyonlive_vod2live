package playout

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisStore struct{ Client *redis.Client }

func (s RedisStore) stateKey(channelID uuid.UUID) string {
	return "vod2live:channel:" + channelID.String() + ":state"
}
func (s RedisStore) lockKey(channelID uuid.UUID) string {
	return "vod2live:channel:" + channelID.String() + ":lock"
}

func (s RedisStore) Get(ctx context.Context, channelID uuid.UUID) (State, error) {
	var state State
	raw, err := s.Client.Get(ctx, s.stateKey(channelID)).Bytes()
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("decode playout state: %w", err)
	}
	return state, nil
}

func (s RedisStore) Put(ctx context.Context, state State, ttl time.Duration) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.Client.Set(ctx, s.stateKey(state.ChannelID), raw, ttl).Err()
}

func (s RedisStore) AcquireLock(ctx context.Context, channelID uuid.UUID, owner string, ttl time.Duration) (bool, error) {
	return s.Client.SetNX(ctx, s.lockKey(channelID), owner, ttl).Result()
}

func (s RedisStore) ReleaseLock(ctx context.Context, channelID uuid.UUID, owner string) error {
	const script = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
	return s.Client.Eval(ctx, script, []string{s.lockKey(channelID)}, owner).Err()
}
