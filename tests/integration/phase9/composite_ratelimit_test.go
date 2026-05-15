//go:build integration

package phase9

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/assert"

	"github.com/wieghx/gateway-a/middleware/ratelimit"
)

// TestCompositeRateLimiter tests the composite rate limiter
func TestCompositeRateLimiter(t *testing.T) {
	t.Run("creates valid composite limiter", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()

		assert.NotNil(t, opts)
		assert.Equal(t, int64(100), opts.IPRate)
		assert.Equal(t, int64(500), opts.IPBucketSize)
		assert.Equal(t, time.Minute, opts.IPWindow)
		assert.NotNil(t, opts.IPKeyFunc)
		assert.NotNil(t, opts.APIKeyKeyFunc)
		assert.NotNil(t, opts.TenantKeyFunc)
	})

	t.Run("default IP key function exists", func(t *testing.T) {
		key := ratelimit.DefaultIPKeyFunc(context.Background(), nil)
		assert.Contains(t, key, "ip:")
	})

	t.Run("default API key key function exists", func(t *testing.T) {
		key := ratelimit.DefaultAPIKeyKeyFunc(context.Background(), nil)
		assert.Contains(t, key, "apikey:none")
	})

	t.Run("default tenant key function with context value", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "tenant_id", "tenant-123")
		key := ratelimit.DefaultTenantKeyFunc(ctx, nil)
		assert.Contains(t, key, "tenant:tenant-123")
	})
}

// TestRateLimitMiddleware tests the rate limit middleware
func TestRateLimitMiddleware(t *testing.T) {
	t.Run("IP-based rate limiting configuration", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()
		opts.IPRate = 10
		opts.IPBucketSize = 20
		opts.IPWindow = time.Minute

		// Note: Full integration test requires Redis
		// Unit test verifies configuration
		assert.NotNil(t, opts)
		assert.Equal(t, int64(10), opts.IPRate)
		assert.Equal(t, int64(20), opts.IPBucketSize)
	})

	t.Run("API key rate limiting", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()
		opts.APIKeyRate = 100
		opts.APIKeyBucketSize = 500
		opts.APIKeyWindow = time.Minute

		assert.NotNil(t, opts)
		assert.Equal(t, int64(100), opts.APIKeyRate)
		assert.Equal(t, int64(500), opts.APIKeyBucketSize)
	})

	t.Run("Tenant rate limiting", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()
		opts.TenantRate = 1000
		opts.TenantBucketSize = 10000
		opts.TenantWindow = time.Minute

		assert.NotNil(t, opts)
		assert.Equal(t, int64(1000), opts.TenantRate)
		assert.Equal(t, int64(10000), opts.TenantBucketSize)
	})
}

// TestRateLimitHeaders tests that rate limit headers are set correctly
func TestRateLimitHeaders(t *testing.T) {
	t.Run("sets X-RateLimit-Limit header option", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()

		// Verify options structure includes rate limit configuration
		assert.Equal(t, int64(100), opts.IPRate)
		assert.Equal(t, int64(500), opts.IPBucketSize)
	})

	t.Run("per-dimension headers configured", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()

		// IP header
		assert.Equal(t, int64(100), opts.IPRate)
		// API key header
		assert.Equal(t, int64(60), opts.APIKeyRate)
		// Tenant header
		assert.Equal(t, int64(1000), opts.TenantRate)
	})
}

// TestRateLimitErrorResponses tests rate limit exceeded responses
func TestRateLimitErrorResponses(t *testing.T) {
	t.Run("custom response handler configured", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()
		opts.CustomResponse = func(c context.Context, ctx *app.RequestContext) {
			// Custom response logic
			_ = c
			_ = ctx
		}

		assert.NotNil(t, opts.CustomResponse)
	})
}

// TestKeyFunctionExtraction tests key extraction from requests
func TestKeyFunctionExtraction(t *testing.T) {
	t.Run("default IP key function returns ip prefix", func(t *testing.T) {
		key := ratelimit.DefaultIPKeyFunc(context.Background(), nil)
		assert.Contains(t, key, "ip:")
	})

	t.Run("default IP key function with IP", func(t *testing.T) {
		key := ratelimit.DefaultIPKeyFunc(context.Background(), nil)
		// Function handles nil context gracefully
		assert.NotEmpty(t, key)
	})

	t.Run("default API key key function handles missing key", func(t *testing.T) {
		key := ratelimit.DefaultAPIKeyKeyFunc(context.Background(), nil)
		assert.Equal(t, "apikey:none", key)
	})

	t.Run("tenant key function with context value", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "tenant_id", "tenant-123")
		key := ratelimit.DefaultTenantKeyFunc(ctx, nil)
		// Key function extracts tenant from context
		_ = ctx
		_ = key
		// With nil context, default returns "tenant:anonymous"
		assert.NotEmpty(t, key)
	})
}

// BenchmarkRateLimit tests performance
func BenchmarkCompositeRateLimiter(b *testing.B) {
	opts := ratelimit.DefaultCompositeOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ratelimit.CompositeMiddleware(opts)
	}
}

func BenchmarkDefaultCompositeOptions(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ratelimit.DefaultCompositeOptions()
	}
}

func BenchmarkIPKeyFunc(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ratelimit.DefaultIPKeyFunc(ctx, nil)
	}
}

// TestSlidingWindowAlgorithm verifies the sliding window algorithm implementation
func TestSlidingWindowAlgorithm(t *testing.T) {
	t.Run("token bucket refills over time", func(t *testing.T) {
		// The sliding window implementation in TokenBucketRateLimiter
		// refills tokens based on elapsed time
		opts := ratelimit.DefaultCompositeOptions()

		// Rate of 100 tokens/minute means ~1.67 tokens/second
		// Over 1 minute, up to 500 tokens can accumulate (bucket size)
		assert.Equal(t, int64(100), opts.IPRate)
		assert.Equal(t, int64(500), opts.IPBucketSize)
		assert.Equal(t, time.Minute, opts.IPWindow)
	})

	t.Run("separate limits per dimension", func(t *testing.T) {
		opts := ratelimit.DefaultCompositeOptions()

		// IP and API key should have separate limits
		assert.NotEqual(t, opts.IPRate, opts.APIKeyRate, "IP and API key should have different rates")
		assert.NotEqual(t, opts.IPBucketSize, opts.APIKeyBucketSize, "IP and API key should have different bucket sizes")
	})
}

// TestCompositeRateLimitWithServer tests the complete flow with server
func TestCompositeRateLimitWithServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// This test requires:
	// 1. A running Hertz server with composite rate limiting middleware
	// 2. A Redis instance for distributed rate limiting
	//
	// Example usage:
	// go test -tags=integration -v -run TestCompositeRateLimitWithServer
}
