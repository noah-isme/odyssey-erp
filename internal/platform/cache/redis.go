package cache

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// New creates a new Redis client.
func New(ctx context.Context, addr string) (*redis.Client, error) {
	options, err := redisOptions(addr)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(options)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("platform/cache: ping: %w", err)
	}

	return client, nil
}

func redisOptions(address string) (*redis.Options, error) {
	if strings.Contains(address, "://") {
		options, err := redis.ParseURL(address)
		if err != nil {
			return nil, fmt.Errorf("platform/cache: parse URL: %w", err)
		}
		return options, nil
	}
	return &redis.Options{Addr: address}, nil
}

// AsynqOptions converts REDIS_ADDR, including redis:// and rediss:// URLs, to Asynq options.
func AsynqOptions(address string) (asynq.RedisClientOpt, error) {
	options, err := redisOptions(address)
	if err != nil {
		return asynq.RedisClientOpt{}, err
	}
	return asynq.RedisClientOpt{
		Addr:      options.Addr,
		Username:  options.Username,
		Password:  options.Password,
		DB:        options.DB,
		TLSConfig: options.TLSConfig,
	}, nil
}
