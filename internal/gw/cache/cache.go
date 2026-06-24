// Package cache is a thin Redis JSON cache used only for hot, regenerable data
// (node discovery list). All methods are nil-safe so the gateway degrades to
// "no cache" rather than failing when Redis is unavailable.
package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a Redis client (which may be nil).
type Cache struct {
	rdb *redis.Client
}

// New connects to Redis and pings it. A connection failure returns a usable
// no-op Cache plus the error, so callers can log and continue.
func New(ctx context.Context, addr, password string) (*Cache, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: password})
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return &Cache{}, err
	}
	return &Cache{rdb: rdb}, nil
}

// GetJSON loads key into dst. Returns (false, nil) on miss or when disabled.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, nil
	}
	b, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return false, err
	}
	return true, nil
}

// SetJSON stores val at key with a TTL. No-op when disabled.
func (c *Cache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, b, ttl).Err()
}

// Del removes keys (e.g. invalidate discovery on node change). No-op when disabled.
func (c *Cache) Del(ctx context.Context, keys ...string) {
	if c == nil || c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, keys...).Err()
}

// Allow is a fixed-window per-key rate limiter (Redis INCR/EXPIRE). It fails
// OPEN (returns true) when Redis is unavailable or the limit is non-positive, so
// rate limiting never takes the gateway down.
func (c *Cache) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	if c == nil || c.rdb == nil || limit <= 0 {
		return true
	}
	n, err := c.rdb.Incr(ctx, key).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		_ = c.rdb.Expire(ctx, key, window).Err()
	}
	return n <= int64(limit)
}

// Ping reports whether Redis is reachable (false when caching is disabled).
func (c *Cache) Ping(ctx context.Context) bool {
	if c == nil || c.rdb == nil {
		return false
	}
	return c.rdb.Ping(ctx).Err() == nil
}

// Close closes the underlying client.
func (c *Cache) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}
