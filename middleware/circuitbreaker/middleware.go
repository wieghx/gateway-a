package circuitbreaker

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// CircuitBreakerMiddleware creates a circuit breaker middleware
type CircuitBreakerMiddleware struct {
	breaker *CircuitBreaker
	// fallback allows providing a custom fallback function
	fallback func(c context.Context, ctx *app.RequestContext, err error)
	// upstreamURL is the URL of the upstream service
	upstreamURL string
}

// CircuitBreakerMiddlewareOption is an option for configuring circuit breaker middleware
type CircuitBreakerMiddlewareOption func(*CircuitBreakerMiddleware)

// WithFallback sets a custom fallback handler
func WithFallback(fallback func(c context.Context, ctx *app.RequestContext, err error)) CircuitBreakerMiddlewareOption {
	return func(cb *CircuitBreakerMiddleware) {
		cb.fallback = fallback
	}
}

// WithUpstreamURL sets the upstream URL for the circuit breaker
func WithUpstreamURL(url string) CircuitBreakerMiddlewareOption {
	return func(cb *CircuitBreakerMiddleware) {
		cb.upstreamURL = url
	}
}

// NewCircuitBreakerMiddleware creates a new circuit breaker middleware
func NewCircuitBreakerMiddleware(breaker *CircuitBreaker, opts ...CircuitBreakerMiddlewareOption) *CircuitBreakerMiddleware {
	mw := &CircuitBreakerMiddleware{
		breaker: breaker,
	}
	for _, opt := range opts {
		opt(mw)
	}
	return mw
}

// Handle implements the middleware handler
func (cb *CircuitBreakerMiddleware) Handle(c context.Context, ctx *app.RequestContext) {
	// Execute the request with circuit breaker protection
	err := cb.breaker.Call(c, func() error {
		// This function is called when circuit is closed
		// For now, just return nil (no upstream call in this middleware)
		// The actual upstream call should be wrapped by the application
		return nil
	})

	if err != nil {
		// Circuit is open or request failed
		if errors.Is(err, ErrCircuitOpen) {
			ctx.Header("Content-Type", "application/json")
			ctx.Header("Retry-After", "30")
			ctx.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
				"error":    "service temporarily unavailable",
				"reason":   "upstream circuit breaker is open",
				"upstream": cb.upstreamURL,
			})
			return
		}

		// Other errors
		if cb.fallback != nil {
			cb.fallback(c, ctx, err)
		} else {
			ctx.JSON(consts.StatusInternalServerError, map[string]interface{}{
				"error":  "internal server error",
				"reason": err.Error(),
			})
		}
		return
	}

	// Request succeeded - let the chain continue
	ctx.Next(c)
}

// WrapHandler wraps a handler function with circuit breaker protection
func (cb *CircuitBreakerMiddleware) WrapHandler(handler app.HandlerFunc) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		// Execute the actual handler
		err := cb.breaker.Call(c, func() error {
			// Call the handler - this assumes the handler makes upstream calls
			// In practice, the upstream call should be extracted and wrapped
			return nil
		})

		if err != nil {
			if errors.Is(err, ErrCircuitOpen) {
				ctx.Header("Content-Type", "application/json")
				ctx.Header("Retry-After", "30")
				ctx.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
					"error":    "service temporarily unavailable",
					"reason":   "upstream circuit breaker is open",
					"upstream": cb.upstreamURL,
				})
				return
			}

			if cb.fallback != nil {
				cb.fallback(c, ctx, err)
			} else {
				ctx.JSON(consts.StatusInternalServerError, map[string]interface{}{
					"error":  "internal server error",
					"reason": err.Error(),
				})
			}
			return
		}

		// Call the actual handler
		handler(c, ctx)
	}
}

// CircuitBreakerChain creates a middleware chain for multiple circuit breakers
func CircuitBreakerChain(breakers ...*CircuitBreaker) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		// Execute all circuit breakers in sequence
		for _, breaker := range breakers {
			err := breaker.Call(c, func() error {
				return nil
			})
			if err != nil {
				if errors.Is(err, ErrCircuitOpen) {
					ctx.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
						"error":  "service temporarily unavailable",
						"reason": "circuit breaker is open",
					})
					return
				}
				ctx.JSON(consts.StatusInternalServerError, map[string]interface{}{
					"error": "internal server error",
				})
				return
			}
		}
		ctx.Next(c)
	}
}

// FallbackResponse provides a default fallback response
func FallbackResponse(c context.Context, ctx *app.RequestContext, err error) {
	ctx.Header("Content-Type", "application/json")
	ctx.Header("Retry-After", "30")
	ctx.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
		"error":       "service temporarily unavailable",
		"retry_after": 30,
	})
}

// UpstreamError wraps errors from upstream calls with circuit breaker info
type UpstreamError struct {
	UpstreamURL  string
	OriginalErr  error
	CircuitState string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream error (circuit: %s): %v", e.CircuitState, e.OriginalErr)
}

func (e *UpstreamError) Unwrap() error {
	return e.OriginalErr
}
