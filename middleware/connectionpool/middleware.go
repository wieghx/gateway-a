package connectionpool

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

// ConnectionPoolMiddleware creates a middleware that configures HTTP clients with connection pooling
type ConnectionPoolMiddleware struct {
	pool *TransportPool
}

// NewConnectionPoolMiddleware creates a new connection pool middleware
func NewConnectionPoolMiddleware(pool *TransportPool) *ConnectionPoolMiddleware {
	return &ConnectionPoolMiddleware{
		pool: pool,
	}
}

// Handle implements the middleware handler
func (mw *ConnectionPoolMiddleware) Handle(c context.Context, ctx *app.RequestContext) {
	// Store the pooled client in the context for use by downstream handlers
	ctx.Set("upstream_client", mw.pool.Client())
	ctx.Next(c)
}

// GetClient retrieves a pooled HTTP client from the context
func GetClient(ctx context.Context) *http.Client {
	if ctx == nil {
		return http.DefaultClient
	}

	// Try to get from Hertz context first
	client := ctx.Value("upstream_client")
	if client != nil {
		if hc, ok := client.(*http.Client); ok {
			return hc
		}
	}

	// Try to get from Go context
	if hc, ok := ctx.Value("upstream_client").(*http.Client); ok {
		return hc
	}

	return http.DefaultClient
}

// WithPool creates a middleware that provides a pooled HTTP client
func WithPool(pool *TransportPool) app.HandlerFunc {
	mw := NewConnectionPoolMiddleware(pool)
	return mw.Handle
}

// PoolFromContext retrieves the connection pool from context
func PoolFromContext(ctx context.Context) *TransportPool {
	if ctx == nil {
		return nil
	}

	if p, ok := ctx.Value("upstream_pool").(*TransportPool); ok {
		return p
	}

	return nil
}

// WithPoolContext creates a middleware that stores the pool in context
func WithPoolContext(pool *TransportPool) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		c = context.WithValue(c, "upstream_pool", pool)
		c = context.WithValue(c, "upstream_client", pool.Client())
		ctx.Next(c)
	}
}

// UpstreamCall executes an HTTP call using the pooled client
func UpstreamCall(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}

	// Set request context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Cancel the request on context cancellation
	req = req.WithContext(timeoutCtx)

	return client.Do(req)
}

// UpstreamCallWithRetry executes an HTTP call with retry logic
func UpstreamCallWithRetry(ctx context.Context, client *http.Client, req *http.Request, maxRetries int, backoff time.Duration) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = UpstreamCall(ctx, client, req)
		if err == nil {
			return resp, nil
		}

		// Don't retry on last attempt
		if attempt == maxRetries {
			break
		}

		// Wait before retry
		select {
		case <-time.After(backoff):
			// Continue to retry
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		// Exponential backoff
		backoff *= 2
	}

	return nil, err
}

// HealthCheck performs a health check on the connection pool
func HealthCheck(ctx context.Context, client *http.Client, url string, timeout time.Duration) error {
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: resp.StatusCode}
	}

	return nil
}

// HTTPError represents an HTTP error
type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	return "HTTP error: " + string(e.Body)
}

// MiddlewareConfig contains middleware configuration
type MiddlewareConfig struct {
	// PoolConfig is the connection pool configuration
	PoolConfig PoolConfig
	// EnableStats enables statistics tracking
	EnableStats bool
}

// DefaultMiddlewareConfig returns default middleware configuration
func DefaultMiddlewareConfig() MiddlewareConfig {
	return MiddlewareConfig{
		PoolConfig:  DefaultPoolConfig(),
		EnableStats: true,
	}
}
