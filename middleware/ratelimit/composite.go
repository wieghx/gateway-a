package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/gu/gateway-a/internal/logging"
)

// CompositeRateLimiter provides multi-dimensional rate limiting
// Supports both IP-based and API key-based rate limits
type CompositeRateLimiter struct {
	ipLimiter      *TokenBucketRateLimiter
	apiKeyLimiter  *TokenBucketRateLimiter
	tenantLimiter  *TokenBucketRateLimiter
	defaultRate    int64
	defaultLimit   int64
	defaultWindow  time.Duration
	redisPrefix    string
}

// CompositeRateLimitResult contains rate limit results for all dimensions
type CompositeRateLimitResult struct {
	Allowed       bool
	IPRemaining   int64
	APIKeyRemaining int64
	TenantRemaining int64
	IPLimit       int64
	APIKeyLimit   int64
	TenantLimit   int64
	RetryAfter    time.Duration
	Reason        string // Which limiter blocked the request
}

// CompositeOptions contains configuration for composite rate limiting
type CompositeOptions struct {
	// IP rate limiting configuration
	IPRate       int64
	IPBucketSize int64
	IPWindow     time.Duration

	// API key rate limiting configuration
	APIKeyRate       int64
	APIKeyBucketSize int64
	APIKeyWindow     time.Duration

	// Tenant rate limiting configuration
	TenantRate       int64
	TenantBucketSize int64
	TenantWindow     time.Duration

	// Custom key functions
	IPKeyFunc      func(c context.Context, ctx *app.RequestContext) string
	APIKeyKeyFunc  func(c context.Context, ctx *app.RequestContext) string
	TenantKeyFunc  func(c context.Context, ctx *app.RequestContext) string

	// CustomResponse allows overriding the default 429 response
	CustomResponse func(c context.Context, ctx *app.RequestContext)

	// Strict mode - fail if ANY dimension is rate limited
	StrictMode bool
}

// DefaultCompositeOptions returns default composite rate limiting configuration
func DefaultCompositeOptions() CompositeOptions {
	return CompositeOptions{
		// IP-based limits (stricter to prevent abuse)
		IPRate:       100,
		IPBucketSize: 500,
		IPWindow:     time.Minute,

		// API key limits (moderate)
		APIKeyRate:       60,
		APIKeyBucketSize: 1000,
		APIKeyWindow:     time.Minute,

		// Tenant limits (per-tenant quotas)
		TenantRate:       1000,
		TenantBucketSize: 10000,
		TenantWindow:     time.Minute,

		// Default key functions
		IPKeyFunc:      DefaultIPKeyFunc,
		APIKeyKeyFunc:  DefaultAPIKeyKeyFunc,
		TenantKeyFunc:  DefaultTenantKeyFunc,

		// Strict mode - if enabled, any rate limit breach blocks the request
		StrictMode: true,
	}
}

// NewCompositeRateLimiter creates a new composite rate limiter
func NewCompositeRateLimiter(redisClient *redis.Client, opts CompositeOptions) *CompositeRateLimiter {
	return &CompositeRateLimiter{
		ipLimiter: NewTokenBucketRateLimiter(redisClient, opts.IPRate, opts.IPBucketSize, opts.IPWindow),
		apiKeyLimiter: NewTokenBucketRateLimiter(redisClient, opts.APIKeyRate, opts.APIKeyBucketSize, opts.APIKeyWindow),
		tenantLimiter: NewTokenBucketRateLimiter(redisClient, opts.TenantRate, opts.TenantBucketSize, opts.TenantWindow),
		defaultRate: opts.IPRate,
		defaultLimit: opts.IPBucketSize,
		defaultWindow: opts.IPWindow,
		redisPrefix: "ratelimit:",
	}
}

// SetPrefix sets the Redis key prefix for all limiters
func (c *CompositeRateLimiter) SetPrefix(prefix string) {
	c.ipLimiter.SetPrefix(prefix)
	c.apiKeyLimiter.SetPrefix(prefix)
	c.tenantLimiter.SetPrefix(prefix)
}

// Check performs rate limit checks across all dimensions
func (c *CompositeRateLimiter) Check(ctx context.Context, keyFuncs []KeyFunc) (*CompositeRateLimitResult, error) {
	result := &CompositeRateLimitResult{
		Allowed: true,
	}

	var blockedBy string

	// Check IP-based limit
	if len(keyFuncs) >= 1 && keyFuncs[0] != nil {
		ipKey := keyFuncs[0](ctx, nil)
		ipResult, err := c.ipLimiter.Check(ctx, ipKey)
		if err != nil {
			// Log error but continue with other checks
			logging.Logger.Warn("IP rate limit check failed",
				logging.WithRequestID(logging.GetRequestID(ctx)),
				zap.String("key", ipKey),
				zap.Error(err),
			)
		} else {
			result.IPRemaining = ipResult.Remaining
			result.IPLimit = ipResult.Limit
			if !ipResult.Allowed {
				blockedBy = "ip"
			}
		}
	}

	// Check API key-based limit
	if len(keyFuncs) >= 2 && keyFuncs[1] != nil {
		apiKeyKey := keyFuncs[1](ctx, nil)
		apiKeyResult, err := c.apiKeyLimiter.Check(ctx, apiKeyKey)
		if err != nil {
			logging.Logger.Warn("API key rate limit check failed",
				logging.WithRequestID(logging.GetRequestID(ctx)),
				zap.String("key", apiKeyKey),
				zap.Error(err),
			)
		} else {
			result.APIKeyRemaining = apiKeyResult.Remaining
			result.APIKeyLimit = apiKeyResult.Limit
			if !apiKeyResult.Allowed {
				blockedBy = "api_key"
			}
		}
	}

	// Check tenant-based limit
	if len(keyFuncs) >= 3 && keyFuncs[2] != nil {
		tenantKey := keyFuncs[2](ctx, nil)
		tenantResult, err := c.tenantLimiter.Check(ctx, tenantKey)
		if err != nil {
			logging.Logger.Warn("Tenant rate limit check failed",
				logging.WithRequestID(logging.GetRequestID(ctx)),
				zap.String("key", tenantKey),
				zap.Error(err),
			)
		} else {
			result.TenantRemaining = tenantResult.Remaining
			result.TenantLimit = tenantResult.Limit
			if !tenantResult.Allowed {
				blockedBy = "tenant"
			}
		}
	}

	// Determine overall result
	if blockedBy != "" {
		result.Allowed = false
		result.Reason = blockedBy
		result.RetryAfter = time.Second
	}

	return result, nil
}

// CheckWithFuncs checks rate limits with specific key functions
func (c *CompositeRateLimiter) CheckWithFuncs(ctx context.Context, ctxFunc app.HandlerFunc, keyFuncs ...KeyFunc) (*CompositeRateLimitResult, error) {
	return c.Check(ctx, keyFuncs)
}

// KeyFunc defines the function signature for extracting rate limit keys
type KeyFunc func(c context.Context, ctx *app.RequestContext) string

// DefaultIPKeyFunc extracts IP address for rate limiting
func DefaultIPKeyFunc(ctx context.Context, rctx *app.RequestContext) string {
	if rctx != nil {
		ip := rctx.ClientIP()
		return fmt.Sprintf("ip:%s", ip)
	}
	return "ip:unknown"
}

// DefaultAPIKeyKeyFunc extracts API key for rate limiting
func DefaultAPIKeyKeyFunc(ctx context.Context, rctx *app.RequestContext) string {
	if rctx != nil {
		authHeader := rctx.Request.Header.Get("Authorization")
		if authHeader != "" {
			apiKey := strings.TrimPrefix(authHeader, "Bearer ")
			return fmt.Sprintf("apikey:%s", apiKey)
		}
	}
	return "apikey:none"
}

// DefaultTenantKeyFunc extracts tenant ID for rate limiting
func DefaultTenantKeyFunc(ctx context.Context, rctx *app.RequestContext) string {
	// Try to get tenant from context first (set by auth middleware)
	tenantID, ok := ctx.Value("tenant_id").(string)
	if ok && tenantID != "" {
		return fmt.Sprintf("tenant:%s", tenantID)
	}

	// Fallback: try to get from header if rctx is available
	if rctx != nil {
		tenantID = rctx.Request.Header.Get("X-Tenant-ID")
		if tenantID != "" {
			return fmt.Sprintf("tenant:%s", tenantID)
		}
	}

	return "tenant:anonymous"
}

// CompositeMiddleware creates a rate limiting middleware with IP + API key checking
func CompositeMiddleware(options CompositeOptions) app.HandlerFunc {
	var redisClient *redis.Client
	// The redis client should be injected via context or configuration
	// For now, we use a nil client (the TokenBucketRateLimiter handles this)

	limiter := NewCompositeRateLimiter(redisClient, options)
	limiter.SetPrefix("ratelimit:")

	return func(ctx context.Context, rctx *app.RequestContext) {
		// Define key functions in order of check priority
		keyFuncs := []KeyFunc{
			options.IPKeyFunc,
			options.APIKeyKeyFunc,
			options.TenantKeyFunc,
		}

		result, err := limiter.Check(ctx, keyFuncs)
		if err != nil {
			logging.Logger.Warn("Composite rate limit check failed, allowing request",
				logging.WithRequestID(logging.GetRequestID(ctx)),
				zap.Error(err),
			)
			rctx.Next(ctx)
			return
		}

		// Set rate limit headers
		rctx.Header("X-RateLimit-Limit", strconv.FormatInt(result.IPLimit, 10))
		rctx.Header("X-RateLimit-Remaining", strconv.FormatInt(result.IPRemaining, 10))
		rctx.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(options.IPWindow).Unix(), 10))

		// Per-dimension headers
		rctx.Header("X-RateLimit-IP-Remaining", strconv.FormatInt(result.IPRemaining, 10))
		rctx.Header("X-RateLimit-APIKey-Remaining", strconv.FormatInt(result.APIKeyRemaining, 10))
		rctx.Header("X-RateLimit-Tenant-Remaining", strconv.FormatInt(result.TenantRemaining, 10))

		if !result.Allowed {
			// Request rate limited
			retryAfter := int64(time.Second)
			if result.RetryAfter > 0 {
				retryAfter = int64(result.RetryAfter.Seconds())
				if retryAfter == 0 {
					retryAfter = 1
				}
			}

			rctx.Header("Retry-After", strconv.FormatInt(retryAfter, 10))

			// Set the blocking reason
			rctx.Header("X-RateLimit-Blocked-By", result.Reason)

			// Custom response or default 429
			if options.CustomResponse != nil {
				options.CustomResponse(ctx, rctx) // Pass nil since we have CompositeRateLimitResult
			} else {
				rctx.JSON(consts.StatusTooManyRequests, map[string]interface{}{
					"error":       "rate limit exceeded",
					"blocked_by":  result.Reason,
					"retry_after": retryAfter,
					"ip_remaining": result.IPRemaining,
					"apikey_remaining": result.APIKeyRemaining,
					"tenant_remaining": result.TenantRemaining,
				})
			}
			return
		}

		// Request allowed
		rctx.Next(ctx)
	}
}

// CompositeMiddlewareWithKeyFuncs creates middleware with custom key extraction
func CompositeMiddlewareWithKeyFuncs(options CompositeOptions, ipKeyFunc, apiKeyFunc, tenantKeyFunc KeyFunc) app.HandlerFunc {
	options.IPKeyFunc = ipKeyFunc
	options.APIKeyKeyFunc = apiKeyFunc
	options.TenantKeyFunc = tenantKeyFunc
	return CompositeMiddleware(options)
}

// GetRateLimitStatus checks current rate limit status without consuming tokens
func (c *CompositeRateLimiter) GetRateLimitStatus(ctx context.Context, keyFuncs []KeyFunc) (*CompositeRateLimitResult, error) {
	result := &CompositeRateLimitResult{
		Allowed: true,
	}

	// Check IP status
	if len(keyFuncs) >= 1 && keyFuncs[0] != nil {
		ipKey := keyFuncs[0](ctx, nil)
		ipTokens, err := c.ipLimiter.GetTokens(ctx, ipKey)
		if err == nil && ipTokens > 0 {
			result.IPRemaining = int64(ipTokens)
			result.IPLimit = c.defaultLimit
		}
	}

	// Check API key status
	if len(keyFuncs) >= 2 && keyFuncs[1] != nil {
		apiKeyKey := keyFuncs[1](ctx, nil)
		apiKeyTokens, err := c.apiKeyLimiter.GetTokens(ctx, apiKeyKey)
		if err == nil && apiKeyTokens > 0 {
			result.APIKeyRemaining = int64(apiKeyTokens)
			result.APIKeyLimit = c.apiKeyLimiter.bucketSize
		}
	}

	// Check tenant status
	if len(keyFuncs) >= 3 && keyFuncs[2] != nil {
		tenantKey := keyFuncs[2](ctx, nil)
		tenantTokens, err := c.tenantLimiter.GetTokens(ctx, tenantKey)
		if err == nil && tenantTokens > 0 {
			result.TenantRemaining = int64(tenantTokens)
			result.TenantLimit = c.tenantLimiter.bucketSize
		}
	}

	return result, nil
}
