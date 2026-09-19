package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps a redis.Client instance.
type Client struct {
	rdb *redis.Client
}

// NewClient initializes a Redis client from a connection URL.
func NewClient(ctx context.Context, redisURL string) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	rdb := redis.NewClient(opt)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		// Return client even if temporarily offline to permit graceful retries
		return &Client{rdb: rdb}, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

// Underlying returns the raw *redis.Client.
func (c *Client) Underlying() *redis.Client {
	return c.rdb
}

// SetTicker caches a live ticker quote with a TTL.
func (c *Client) SetTicker(ctx context.Context, symbol string, quote *TickerQuote, ttl time.Duration) error {
	data, err := quote.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal ticker: %w", err)
	}
	return c.rdb.Set(ctx, TickerKey(symbol), data, ttl).Err()
}

// GetTicker fetches a cached ticker quote.
func (c *Client) GetTicker(ctx context.Context, symbol string) (*TickerQuote, error) {
	data, err := c.rdb.Get(ctx, TickerKey(symbol)).Bytes()
	if err != nil {
		return nil, err
	}
	var quote TickerQuote
	if err := quote.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ticker: %w", err)
	}
	return &quote, nil
}

// Publish broadcasts an event payload onto a pub/sub channel.
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) error {
	return c.rdb.Publish(ctx, channel, message).Err()
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	if c.rdb != nil {
		return c.rdb.Close()
	}
	return nil
}
