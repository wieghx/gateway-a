package security

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// SecurityHeaders contains security header configuration
type SecurityHeaders struct {
	// ContentSecurityPolicy sets the Content-Security-Policy header
	ContentSecurityPolicy string
	// XFrameOptions sets the X-Frame-Options header (DENY, SAMEORIGIN, or ALLOW-FROM uri)
	XFrameOptions string
	// XContentTypeOptions sets the X-Content-Type-Options header (should be "nosniff")
	XContentTypeOptions string
	// XXSSProtection sets the X-XSS-Protection header
	XXSSProtection string
	// XPermittedCrossDomainPolicies sets the X-Permitted-Cross-Domain-Policies header
	XPermittedCrossDomainPolicies string
	// ReferrerPolicy sets the Referrer-Policy header
	ReferrerPolicy string
	// PermissionsPolicy sets the Permissions-Policy header
	PermissionsPolicy string
	// StrictTransportSecurity sets the Strict-Transport-Security header (e.g., "max-age=31536000; includeSubDomains")
	StrictTransportSecurity string
}

// DefaultSecurityHeaders returns OWASP-recommended default security headers
func DefaultSecurityHeaders() SecurityHeaders {
	return SecurityHeaders{
		XFrameOptions:            "DENY",
		XContentTypeOptions:      "nosniff",
		XXSSProtection:           "0",
		XPermittedCrossDomainPolicies: "none",
		ReferrerPolicy:           "strict-origin-when-cross-origin",
		PermissionsPolicy:        "camera=(), microphone=(), geolocation=()",
		StrictTransportSecurity:  "max-age=31536000; includeSubDomains; preload",
	}
}

// CORSOptions contains CORS configuration
type CORSOptions struct {
	// AllowedOrigins is the list of allowed origins
	AllowedOrigins []string
	// AllowedMethods is the list of allowed HTTP methods
	AllowedMethods []string
	// AllowedHeaders is the list of allowed headers
	AllowedHeaders []string
	// ExposedHeaders is the list of exposed response headers
	ExposedHeaders []string
	// AllowCredentials indicates if credentials are allowed
	AllowCredentials bool
	// MaxAge is the cache duration for preflight requests
	MaxAge int
}

// DefaultCORSOptions returns default CORS configuration
func DefaultCORSOptions() CORSOptions {
	return CORSOptions{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Trace-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "X-Trace-ID"},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// Security creates a security headers middleware
func Security(headers SecurityHeaders) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		// Set security headers
		if headers.XFrameOptions != "" {
			ctx.Header("X-Frame-Options", headers.XFrameOptions)
		}
		if headers.XContentTypeOptions != "" {
			ctx.Header("X-Content-Type-Options", headers.XContentTypeOptions)
		}
		if headers.XXSSProtection != "" {
			ctx.Header("X-XSS-Protection", headers.XXSSProtection)
		}
		if headers.XPermittedCrossDomainPolicies != "" {
			ctx.Header("X-Permitted-Cross-Domain-Policies", headers.XPermittedCrossDomainPolicies)
		}
		if headers.ReferrerPolicy != "" {
			ctx.Header("Referrer-Policy", headers.ReferrerPolicy)
		}
		if headers.PermissionsPolicy != "" {
			ctx.Header("Permissions-Policy", headers.PermissionsPolicy)
		}
		if headers.StrictTransportSecurity != "" {
			ctx.Header("Strict-Transport-Security", headers.StrictTransportSecurity)
		}
		if headers.ContentSecurityPolicy != "" {
			ctx.Header("Content-Security-Policy", headers.ContentSecurityPolicy)
		}

		ctx.Next(c)
	}
}

// CORS creates a CORS middleware
func CORS(opts CORSOptions) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		origin := string(ctx.GetHeader("Origin"))
		allowedOrigin := false

		// Check if origin is allowed
		if len(opts.AllowedOrigins) > 0 {
			for _, allowed := range opts.AllowedOrigins {
				if allowed == "*" || allowed == origin {
					allowedOrigin = true
					if allowed != "*" {
						ctx.Header("Vary", "Origin")
						ctx.Header("Access-Control-Allow-Origin", origin)
					} else {
						ctx.Header("Access-Control-Allow-Origin", "*")
					}
					break
				}
			}
		} else {
			// Default: allow all origins
			allowedOrigin = true
			ctx.Header("Access-Control-Allow-Origin", "*")
		}

		// Handle preflight requests
		if string(ctx.Method()) == "OPTIONS" {
			// Set allowed methods
			if len(opts.AllowedMethods) > 0 {
				ctx.Header("Access-Control-Allow-Methods", strings.Join(opts.AllowedMethods, ", "))
			}

			// Set allowed headers
			if len(opts.AllowedHeaders) > 0 {
				ctx.Header("Access-Control-Allow-Headers", strings.Join(opts.AllowedHeaders, ", "))
			}

			// Set exposed headers
			if len(opts.ExposedHeaders) > 0 {
				ctx.Header("Access-Control-Expose-Headers", strings.Join(opts.ExposedHeaders, ", "))
			}

			// Set max age
			if opts.MaxAge > 0 {
				ctx.Header("Access-Control-Max-Age", strconv.Itoa(opts.MaxAge))
			}

			// Set allow credentials
			if opts.AllowCredentials {
				ctx.Header("Access-Control-Allow-Credentials", "true")
			}

			// Return 204 No Content for preflight
			ctx.SetStatusCode(http.StatusNoContent)
			ctx.SetBodyString("")
			ctx.Abort()
			return
		}

		// Set allow credentials for actual requests
		if opts.AllowCredentials && allowedOrigin {
			ctx.Header("Access-Control-Allow-Credentials", "true")
		}

		ctx.Next(c)
	}
}
