package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// TokenBucketRateLimiter implements a distributed token bucket rate limiter with sliding window
type TokenBucketRateLimiter struct {
	client        *redis.Client
	rate          int64        // tokens per second
	bucketSize    int64        // max bucket size
	defaultWindow time.Duration // sliding window duration
	redisPrefix   string
	mu            sync.RWMutex
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	Allowed       bool
	Remaining     int64
	ResetAt       time.Time
	Limit         int64
	RetryAfter    time.Duration
	CurrentTokens float64
}

// NewTokenBucketRateLimiter creates a new distributed token bucket rate limiter
func NewTokenBucketRateLimiter(client *redis.Client, rate int64, bucketSize int64, window time.Duration) *TokenBucketRateLimiter {
	return &TokenBucketRateLimiter{
		client:        client,
		rate:          rate,
		bucketSize:    bucketSize,
		defaultWindow: window,
		redisPrefix:   "ratelimit:",
	}
}

// SetPrefix sets the Redis key prefix for rate limiting
func (r *TokenBucketRateLimiter) SetPrefix(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.redisPrefix = prefix
}

// Check determines if a request is allowed under the rate limit
func (r *TokenBucketRateLimiter) Check(ctx context.Context, key string) (*RateLimitResult, error) {
	// Get current time
	now := time.Now()

	// Lua script for atomic token bucket with sliding window
	script := `
	local key = KEYS[1]
	local rate = tonumber(ARGV[1])
	local capacity = tonumber(ARGV[2])
	local now = tonumber(ARGV[3])
	local window = tonumber(ARGV[4])
	local requested = tonumber(ARGV[5])

	-- Get current bucket state
	local bucket = redis.call('HMGET', key, 'tokens', 'last_update')
	local tokens = tonumber(bucket[1])
	local last_update = tonumber(bucket[2])

	-- Initialize if not exists
	if tokens == nil then
		tokens = capacity
		last_update = now
	end

	-- Calculate tokens to add based on time elapsed (elapsed is ms, rate is tokens/sec)
	local elapsed = now - last_update
	local tokensToAdd = (elapsed / 1000.0) * rate
	tokens = math.min(capacity, tokens + tokensToAdd)

	-- Check if request is allowed
	local allowed = 0
	if tokens >= requested then
		tokens = tokens - requested
		allowed = 1
	end

	-- Update bucket state
	redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
	redis.call('EXPIRE', key, math.ceil(window / 1000))

	-- Calculate reset time (when bucket will be full)
	local timeToFull = (capacity - tokens) / rate
	local resetAt = now + (timeToFull * 1000)

	-- Calculate retry after if not allowed
	local retryAfter = 0
	if allowed == 0 then
		retryAfter = (requested - tokens) / rate * 1000
	end

	return {allowed, tokens, resetAt, retryAfter}
`

	// Execute Lua script
	result, err := r.client.Eval(ctx, script, []string{r.redisPrefix + key},
		r.rate,
		r.bucketSize,
		float64(now.UnixMilli()),
		float64(r.defaultWindow.Milliseconds()),
		1, // requested tokens
	).Result()
	if err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	// Parse result
	results := result.([]interface{})
	allowed := results[0].(int64) == 1
	remaining := int64(results[1].(float64))
	resetAtMs := int64(results[2].(float64))
	retryAfterMs := int64(results[3].(float64))

	resetAt := time.UnixMilli(resetAtMs)
	retryAfter := time.Duration(retryAfterMs) * time.Millisecond

	return &RateLimitResult{
		Allowed:       allowed,
		Remaining:     remaining,
		ResetAt:       resetAt,
		Limit:         r.bucketSize,
		RetryAfter:    retryAfter,
		CurrentTokens: results[1].(float64),
	}, nil
}

// CheckWithConfig allows custom configuration per request (non-mutating)
func (r *TokenBucketRateLimiter) CheckWithConfig(ctx context.Context, key string, rate, capacity int64, window time.Duration) (*RateLimitResult, error) {
	// Create a temporary limiter instance to avoid mutating shared state (was a data race)
	temp := &TokenBucketRateLimiter{
		client:        r.client,
		rate:          rate,
		bucketSize:    capacity,
		defaultWindow: window,
		redisPrefix:   r.redisPrefix,
	}
	return temp.Check(ctx, key)
}

// Reset resets the rate limit bucket for a key
func (r *TokenBucketRateLimiter) Reset(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.redisPrefix+key).Err()
}

// GetTokens returns the current token count for a key
func (r *TokenBucketRateLimiter) GetTokens(ctx context.Context, key string) (float64, error) {
	script := `
	local key = KEYS[1]
	local now = tonumber(ARGV[1])
	local rate = tonumber(ARGV[2])
	local last_update = tonumber(ARGV[3])

	local bucket = redis.call('HMGET', key, 'tokens', 'last_update')
	local tokens = tonumber(bucket[1])
	local stored_update = tonumber(bucket[2])

	if tokens == nil then
		return nil
	end

	if stored_update ~= last_update then
		local elapsed = now - stored_update
		tokens = math.min(ARGV[4], tokens + (elapsed / 1000.0) * rate)
		redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
	end

	return tokens
`
	result, err := r.client.Eval(ctx, script, []string{r.redisPrefix + key},
		float64(time.Now().UnixMilli()),
		r.rate,
		float64(time.Now().UnixMilli()),
		float64(r.bucketSize),
	).Result()
	if err != nil {
		return 0, fmt.Errorf("get tokens failed: %w", err)
	}

	if result == nil {
		return 0, nil
	}

	return result.(float64), nil
}
