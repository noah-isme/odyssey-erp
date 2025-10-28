package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	cacheVersionKey = "analytics:version"
	bumpChannel     = "gl.bump"
)

// Cache wraps Redis based caching with versioning controls.
type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewCache instantiates the cache helper.
func NewCache(client *redis.Client, ttl time.Duration) *Cache {
	return &Cache{client: client, ttl: ttl}
}

// Version returns the current cache version, initialising when missing.
func (c *Cache) Version(ctx context.Context) (int64, error) {
	if c == nil || c.client == nil {
		return 0, nil
	}
	ver, err := c.client.Get(ctx, cacheVersionKey).Int64()
	if err == redis.Nil {
		if err := c.client.Set(ctx, cacheVersionKey, 1, 0).Err(); err != nil {
			return 0, err
		}
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if ver <= 0 {
		ver = 1
		if err := c.client.Set(ctx, cacheVersionKey, ver, 0).Err(); err != nil {
			return 0, err
		}
	}
	return ver, nil
}

// BuildKey composes the cache key with the current version.
func (c *Cache) BuildKey(ctx context.Context, parts ...string) (string, error) {
	if c == nil || c.client == nil {
		return strings.Join(parts, ":"), nil
	}
	ver, err := c.Version(ctx)
	if err != nil {
		return "", err
	}
	joined := strings.Join(parts, ":")
	return fmt.Sprintf("%s:%d", joined, ver), nil
}

// FetchJSON loads a cached value or populates it using the loader.
func (c *Cache) FetchJSON(ctx context.Context, key string, dest interface{}, loader func(context.Context) (interface{}, error)) error {
	if loader == nil {
		return errors.New("cache: loader required")
	}
	if c == nil || c.client == nil {
		value, err := loader(ctx)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, dest)
	}
	payload, err := c.client.Get(ctx, key).Bytes()
	if err == nil {
		return json.Unmarshal(payload, dest)
	}
	if err != redis.Nil {
		return err
	}
	value, err := loader(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := c.client.Set(ctx, key, raw, c.ttl).Err(); err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}

// Bump invalidates the cache by incrementing the global version and publishing an event.
func (c *Cache) Bump(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	ver, err := c.client.Incr(ctx, cacheVersionKey).Result()
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, bumpChannel, strconv.FormatInt(ver, 10)).Err()
}

// ListenForInvalidation subscribes to version bump notifications.
func (c *Cache) ListenForInvalidation(ctx context.Context, channel string) error {
	if c == nil || c.client == nil {
		return nil
	}
	if channel == "" {
		channel = bumpChannel
	}
	pubsub := c.client.Subscribe(ctx, channel)
	go func() {
		defer func() { _ = pubsub.Close() }()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg.Payload != "" {
					if ver, err := strconv.ParseInt(msg.Payload, 10, 64); err == nil {
						_ = c.client.Set(ctx, cacheVersionKey, ver, 0).Err()
						continue
					}
				}
				_ = c.client.Incr(ctx, cacheVersionKey).Err()
			}
		}
	}()
	return nil
}

func keyKPI(companyID int64, branchID *int64, period string) string {
	return strings.Join([]string{"analytics", "kpi", formatInt(companyID), branchToken(branchID), period}, ":")
}

func keyPLTrend(companyID int64, branchID *int64, from, to string) string {
	return strings.Join([]string{"analytics", "pl_trend", formatInt(companyID), branchToken(branchID), from, to}, ":")
}

func keyCashflow(companyID int64, branchID *int64, from, to string) string {
	return strings.Join([]string{"analytics", "cashflow", formatInt(companyID), branchToken(branchID), from, to}, ":")
}

func keyAging(prefix string, companyID int64, branchID *int64, asOf time.Time) string {
	return strings.Join([]string{"analytics", prefix, formatInt(companyID), branchToken(branchID), asOf.Format("2006-01-02")}, ":")
}
