package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/gu/gateway-a/internal/logging"
)

// MiddlewareOptions contains rate limit middleware configuration
type MiddlewareOptions struct {
	// Rate is the number of tokens added per second
	Rate int64
	// BucketSize is the maximum capacity of the token bucket
	BucketSize int64
	// Window is the sliding window duration
	Window time.Duration
	// KeyFunc extracts a unique key from the request (e.g., IP, API key)
	KeyFunc func(c context.Context, ctx *app.RequestContext) string
	// CustomResponse allows overriding the default 429 response
	CustomResponse func(c context.Context, ctx *app.RequestContext, result *RateLimitResult)
}

// DefaultMiddlewareOptions returns the default rate limit configuration
func DefaultMiddlewareOptions() MiddlewareOptions {
	return MiddlewareOptions{
		Rate:       10,        // 10 tokens per second
		BucketSize: 100,       // Max 100 tokens
		Window:     time.Minute, // 1 minute sliding window
		KeyFunc:    DefaultKeyFunc,
	}
}

// DefaultKeyFunc returns a key based on client IP address
func DefaultKeyFunc(c context.Context, ctx *app.RequestContext) string {
	ip := ctx.ClientIP()
	return fmt.Sprintf("ip:%s", ip)
}

// Middleware creates a rate limiting middleware
func Middleware(options MiddlewareOptions) app.HandlerFunc {
	// Create rate limiter
	limiter := NewTokenBucketRateLimiter(nil, options.Rate, options.BucketSize, options.Window)

	return func(c context.Context, ctx *app.RequestContext) {
		// Extract key for rate limiting
		key := options.KeyFunc(c, ctx)

		// Check rate limit
		result, err := limiter.Check(c, key)
		if err != nil {
			// If rate limiter check fails, allow request but log warning
			logging.Logger.Warn("Rate limiter check failed, allowing request",
				logging.WithRequestID(logging.GetRequestID(c)),
				zap.String("error", err.Error()),
			)
			ctx.Next(c)
			return
		}

		// Set rate limit headers
		ctx.Header("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
		ctx.Header("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
		ctx.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))

		if !result.Allowed {
			// Request rate limited
			retryAfter := result.RetryAfter / time.Second
			if retryAfter == 0 {
				retryAfter = 1
			}
			ctx.Header("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds()), 10))

			// Custom response or default 429
			if options.CustomResponse != nil {
				options.CustomResponse(c, ctx, result)
			} else {
				ctx.JSON(consts.StatusTooManyRequests, map[string]interface{}{
					"error":       "rate limit exceeded",
					"retry_after": retryAfter,
					"limit":       result.Limit,
					"remaining":   0,
				})
			}
			return
		}

		// Request allowed
		ctx.Next(c)
	}
}

// MiddlewareWithRedis creates a rate limiting middleware with Redis client
func MiddlewareWithRedis(client interface{}, options MiddlewareOptions) app.HandlerFunc {
	redisClient, ok := client.(*RedisClientWrapper)
	if !ok {
		// Fallback to in-memory rate limiter
		return Middleware(options)
	}

	// Create rate limiter with Redis
	limiter := NewTokenBucketRateLimiter(redisClient.Client(), options.Rate, options.BucketSize, options.Window)

	return func(c context.Context, ctx *app.RequestContext) {
		// Extract key for rate limiting
		key := options.KeyFunc(c, ctx)

		// Check rate limit
		result, err := limiter.Check(c, key)
		if err != nil {
			// If rate limiter check fails, allow request but log warning
			logging.Logger.Warn("Rate limiter check failed, allowing request",
				logging.WithRequestID(logging.GetRequestID(c)),
				zap.String("error", err.Error()),
			)
			ctx.Next(c)
			return
		}

		// Set rate limit headers
		ctx.Header("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
		ctx.Header("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
		ctx.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))

		if !result.Allowed {
			// Request rate limited
			retryAfter := result.RetryAfter / time.Second
			if retryAfter == 0 {
				retryAfter = 1
			}
			ctx.Header("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds()), 10))

			// Custom response or default 429
			if options.CustomResponse != nil {
				options.CustomResponse(c, ctx, result)
			} else {
				ctx.JSON(consts.StatusTooManyRequests, map[string]interface{}{
					"error":       "rate limit exceeded",
					"retry_after": retryAfter,
					"limit":       result.Limit,
					"remaining":   0,
				})
			}
			return
		}

		// Request allowed
		ctx.Next(c)
	}
}

// WrapRedisClient creates a wrapper for Redis client
type RedisClientWrapper struct {
	client *redis.Client
}

// Client returns the underlying Redis client
func (w *RedisClientWrapper) Client() *redis.Client {
	return w.client
}

// NewRedisClientWrapper creates a wrapper for a Redis client
func NewRedisClientWrapper(client *redis.Client) *RedisClientWrapper {
	return &RedisClientWrapper{client: client}
}
