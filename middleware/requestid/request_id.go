package requestid

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/gu/gateway-a/internal/logging"
)

const (
	// RequestIDHeader is the header key for request ID
	RequestIDHeader = "X-Request-ID"

	// XTraceIDHeader is the header key for trace ID (for distributed tracing)
	XTraceIDHeader = "X-Trace-ID"

	// XRequestIDHeader is an alternative header key for request ID
	XRequestIDHeader = "X-Request-Id"
)

// Generator generates unique request IDs
type Generator interface {
	Generate() string
}

// DefaultGenerator generates random request IDs
type DefaultGenerator struct {
	source *rand.Rand
}

// NewDefaultGenerator creates a new default generator
func NewDefaultGenerator() *DefaultGenerator {
	return &DefaultGenerator{
		source: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Generate creates a new unique request ID
func (g *DefaultGenerator) Generate() string {
	// Generate a random hex string (16 chars = 64 bits)
	return fmt.Sprintf("%x", g.source.Uint64())
}

// Middleware creates a request ID middleware
func Middleware(generator Generator) app.HandlerFunc {
	if generator == nil {
		generator = NewDefaultGenerator()
	}

	return func(c context.Context, ctx *app.RequestContext) {
		// Check if request ID is already in the request
		requestID := string(ctx.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = string(ctx.GetHeader(XTraceIDHeader))
		}
		if requestID == "" {
			requestID = string(ctx.GetHeader(XRequestIDHeader))
		}

		if requestID == "" {
			// Generate new request ID
			requestID = generator.Generate()
		}

		// Set request ID in context
		c = logging.SetRequestID(c, requestID)

		// Add request ID to response headers
		ctx.Header(RequestIDHeader, requestID)
		ctx.Header(XTraceIDHeader, requestID)

		// Proceed with request
		ctx.Next(c)
	}
}

// GetRequestIDFromContext extracts request ID from context
func GetRequestIDFromContext(c context.Context) string {
	return logging.GetRequestID(c)
}

// WithRequestIDMiddleware returns a middleware that ensures request ID is present
func WithRequestIDMiddleware() app.HandlerFunc {
	gen := NewDefaultGenerator()
	return Middleware(gen)
}

// PropagateRequestID propagates request ID through an HTTP call
func PropagateRequestID(ctx context.Context, headers map[string]string) {
	requestID := GetRequestIDFromContext(ctx)
	if requestID != "" {
		headers[RequestIDHeader] = requestID
		headers[XTraceIDHeader] = requestID
	}
}

// CleanRequestID sanitizes a request ID (only alphanumeric and hyphens)
func CleanRequestID(id string) string {
	var builder strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
