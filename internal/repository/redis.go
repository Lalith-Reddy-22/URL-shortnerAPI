package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lalith/urlshortener/internal/model"
	"github.com/redis/go-redis/v9"
)

const linkKeyPrefix = "link:"

// Redis implements CacheStore with go-redis.
type Redis struct {
	client *redis.Client
}

func NewRedis(addr, password string, db int) *Redis {
	return &Redis{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *Redis) Close() error {
	return r.client.Close()
}

func (r *Redis) GetLink(ctx context.Context, code string) (model.CachedLink, error) {
	raw, err := r.client.Get(ctx, linkKey(code)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return model.CachedLink{}, ErrNotFound
		}
		return model.CachedLink{}, err
	}
	var cached model.CachedLink
	if err := json.Unmarshal(raw, &cached); err != nil {
		return model.CachedLink{}, fmt.Errorf("cache decode: %w", err)
	}
	return cached, nil
}

func (r *Redis) SetLink(ctx context.Context, code string, link model.CachedLink, ttl time.Duration) error {
	raw, err := json.Marshal(link)
	if err != nil {
		return fmt.Errorf("cache encode: %w", err)
	}
	return r.client.Set(ctx, linkKey(code), raw, ttl).Err()
}

func (r *Redis) DeleteLink(ctx context.Context, code string) error {
	return r.client.Del(ctx, linkKey(code)).Err()
}

func linkKey(code string) string {
	return linkKeyPrefix + code
}

var _ CacheStore = (*Redis)(nil)
var _ Pinger = (*Redis)(nil)
