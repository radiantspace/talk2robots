package redis

import (
	"context"
	"talk2robots/m/v2/app/config"
	"talk2robots/m/v2/app/util"
	"time"

	r "github.com/go-redis/redis/v8"
)

// Client is a redis client
type Client interface {
	Del(ctx context.Context, keys ...string) *r.IntCmd
	Get(ctx context.Context, key string) *r.StringCmd
	IncrBy(ctx context.Context, key string, value int64) *r.IntCmd
	IncrByFloat(ctx context.Context, key string, value float64) *r.FloatCmd
	Keys(ctx context.Context, pattern string) *r.StringSliceCmd
	Ping(ctx context.Context) *r.StatusCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *r.StatusCmd
}

var RedisClient Client

// NewClient creates a new redis client
func NewClient(cfg config.Redis) Client {
	client := r.NewClient(&r.Options{
		Addr:     cfg.Host + ":" + cfg.Port,
		Password: cfg.Password,
		DB:       0,
	})
	_, err := client.Ping(context.TODO()).Result()
	util.Assert(err == nil, "Redis connection failed", err)
	return client
}

// Define a function to wrap another function in Redis cache.
func WrapInCache(c Client, key string, duration time.Duration, fn func() (string, error)) func() (string, error) {
	return func() (string, error) {
		cachedData, err := c.Get(context.Background(), key).Result()
		if err == nil {
			return cachedData, nil
		}
		// Cache miss or Redis error. Call the original function.
		data, err := fn()
		if err != nil {
			return "", err
		}
		err = c.Set(context.Background(), key, data, duration).Err()
		if err != nil {
			return "", err
		}
		return data, nil
	}
}
