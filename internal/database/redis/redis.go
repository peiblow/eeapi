package redis

import (
	"context"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Client struct {
	*goredis.Client
}

func Open() (*Client, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	rdb := goredis.NewClient(&goredis.Options{Addr: addr})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Client{rdb}, nil
}

func (c *Client) Close() error {
	return c.Client.Close()
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.Client.Set(ctx, key, value, expiration).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.Client.Get(ctx, key).Result()
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.Client.Del(ctx, keys...).Err()
}
