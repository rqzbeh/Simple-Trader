package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

const (
	// MaxFailedAttempts defines the maximum number of failed logins within the window.
	MaxFailedAttempts = 5
	// RateLimitWindow is the duration of the sliding window (15 minutes).
	RateLimitWindow = 15 * time.Minute
)

// RateLimiter tracks login failures per client key (typically remote IP address).
type RateLimiter struct {
	redisClient *cache.Client
	mu          sync.Mutex
	failures    map[string][]time.Time
}

// NewRateLimiter creates a thread-safe sliding window rate limiter.
func NewRateLimiter(redisClient *cache.Client) *RateLimiter {
	rl := &RateLimiter{
		redisClient: redisClient,
		failures:    make(map[string][]time.Time),
	}
	return rl
}

// IsAllowed checks whether the IP key is permitted to attempt login.
// Returns allowed bool, and remaining attempts.
func (rl *RateLimiter) IsAllowed(ctx context.Context, key string) (bool, int) {
	now := time.Now()
	cutoff := now.Add(-RateLimitWindow)

	// In-memory sliding window
	rl.mu.Lock()
	defer rl.mu.Unlock()

	times := rl.failures[key]
	var valid []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rl.failures[key] = valid

	failedCount := len(valid)
	if failedCount >= MaxFailedAttempts {
		return false, 0
	}

	remaining := MaxFailedAttempts - failedCount
	return true, remaining
}

// RecordFailure notes a failed authentication attempt.
func (rl *RateLimiter) RecordFailure(ctx context.Context, key string) {
	now := time.Now()

	rl.mu.Lock()
	rl.failures[key] = append(rl.failures[key], now)
	rl.mu.Unlock()

	if rl.redisClient != nil && rl.redisClient.Underlying() != nil {
		redisKey := fmt.Sprintf("auth:fail:%s", key)
		_ = rl.redisClient.Underlying().Incr(ctx, redisKey).Err()
		_ = rl.redisClient.Underlying().Expire(ctx, redisKey, RateLimitWindow).Err()
	}
}

// Reset clears failure history on successful login.
func (rl *RateLimiter) Reset(ctx context.Context, key string) {
	rl.mu.Lock()
	delete(rl.failures, key)
	rl.mu.Unlock()

	if rl.redisClient != nil && rl.redisClient.Underlying() != nil {
		redisKey := fmt.Sprintf("auth:fail:%s", key)
		_ = rl.redisClient.Underlying().Del(ctx, redisKey).Err()
	}
}
