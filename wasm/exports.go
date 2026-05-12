package wasm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// GoExports provides functions that can be called from WASM plugins.
// These are exported from Go and imported into the WASM module.
type GoExports struct {
	// LogFunc is called by plugins to log messages
	LogFunc func(ctx context.Context, level string, message string)
	// GetHeaderFunc is called by plugins to get HTTP headers
	GetHeaderFunc func(ctx context.Context, name string) string
	// SetHeaderFunc is called by plugins to set HTTP headers
	SetHeaderFunc func(ctx context.Context, name string, value string)
	// GetBodyFunc is called by plugins to get request body
	GetBodyFunc func(ctx context.Context) []byte
	// SetBodyFunc is called by plugins to set response body
	SetBodyFunc func(ctx context.Context, body []byte)
	// GetMetadataFunc is called by plugins to get metadata
	GetMetadataFunc func(ctx context.Context, key string) string
	// SetMetadataFunc is called by plugins to set metadata
	SetMetadataFunc func(ctx context.Context, key string, value string)
	// GetTenantFunc is called by plugins to get tenant ID
	GetTenantFunc func(ctx context.Context) string
	// GetAPIKeyFunc is called by plugins to get API key
	GetAPIKeyFunc func(ctx context.Context) string
	// ValidateAPIKeyFunc is called by plugins to validate API keys
	ValidateAPIKeyFunc func(ctx context.Context, apiKey string) bool
	// RateLimitFunc is called by plugins to check rate limits
	RateLimitFunc func(ctx context.Context, tenantID string, limit int) bool
}

// DefaultLogFunc is the default logging implementation.
func DefaultLogFunc(ctx context.Context, level string, message string) {
	fmt.Printf("[%s] %s\n", level, message)
}

// DefaultGetHeaderFunc returns the default header getter.
func DefaultGetHeaderFunc(ctx context.Context, name string) string {
	input, ok := ctx.Value("pluginInput").(*PluginInput)
	if !ok {
		return ""
	}
	return input.Headers[name]
}

// DefaultSetHeaderFunc sets the default header setter.
func DefaultSetHeaderFunc(ctx context.Context, name string, value string) {
	output, ok := ctx.Value("pluginOutput").(*PluginOutput)
	if !ok {
		return
	}
	if output.Headers == nil {
		output.Headers = make(map[string]string)
	}
	output.Headers[name] = value
}

// DefaultGetBodyFunc returns the default body getter.
func DefaultGetBodyFunc(ctx context.Context) []byte {
	input, ok := ctx.Value("pluginInput").(*PluginInput)
	if !ok {
		return nil
	}
	return input.Body
}

// DefaultSetBodyFunc sets the default body setter.
func DefaultSetBodyFunc(ctx context.Context, body []byte) {
	output, ok := ctx.Value("pluginOutput").(*PluginOutput)
	if !ok {
		return
	}
	output.Body = body
}

// DefaultGetMetadataFunc returns the default metadata getter.
func DefaultGetMetadataFunc(ctx context.Context, key string) string {
	input, ok := ctx.Value("pluginInput").(*PluginInput)
	if !ok {
		return ""
	}
	if input.Context == nil {
		return ""
	}
	if val, ok := input.Context[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// DefaultSetMetadataFunc sets the default metadata setter.
func DefaultSetMetadataFunc(ctx context.Context, key string, value string) {
	input, ok := ctx.Value("pluginInput").(*PluginInput)
	if !ok {
		return
	}
	if input.Context == nil {
		input.Context = make(map[string]interface{})
	}
	input.Context[key] = value
}

// DefaultGetTenantFunc returns the default tenant getter.
func DefaultGetTenantFunc(ctx context.Context) string {
	input, ok := ctx.Value("pluginInput").(*PluginInput)
	if !ok {
		return ""
	}
	return input.TenantID
}

// DefaultGetAPIKeyFunc returns the default API key getter.
func DefaultGetAPIKeyFunc(ctx context.Context) string {
	input, ok := ctx.Value("pluginInput").(*PluginInput)
	if !ok {
		return ""
	}
	return input.APIKey
}

// DefaultValidateAPIKeyFunc returns the default API key validator.
func DefaultValidateAPIKeyFunc(ctx context.Context, apiKey string) bool {
	_ = apiKey
	return true
}

// DefaultRateLimitFunc returns the default rate limiter.
func DefaultRateLimitFunc(ctx context.Context, tenantID string, limit int) bool {
	_ = tenantID
	_ = limit
	return true
}

// NewGoExports creates a new GoExports instance with default implementations.
func NewGoExports() *GoExports {
	return &GoExports{
		LogFunc:          DefaultLogFunc,
		GetHeaderFunc:    DefaultGetHeaderFunc,
		SetHeaderFunc:    DefaultSetHeaderFunc,
		GetBodyFunc:      DefaultGetBodyFunc,
		SetBodyFunc:      DefaultSetBodyFunc,
		GetMetadataFunc:  DefaultGetMetadataFunc,
		SetMetadataFunc:  DefaultSetMetadataFunc,
		GetTenantFunc:    DefaultGetTenantFunc,
		GetAPIKeyFunc:    DefaultGetAPIKeyFunc,
		ValidateAPIKeyFunc: DefaultValidateAPIKeyFunc,
		RateLimitFunc:    DefaultRateLimitFunc,
	}
}

// ExportToWASM exports Go functions as WASM imports.
func (e *GoExports) ExportToWASM(r wazero.Runtime, ctx context.Context, moduleName string) (api.Module, error) {
	// Create host module builder
	builder := r.NewHostModuleBuilder(moduleName)

	// log(level, message) -> void
	builder.NewFunctionBuilder().
		WithFunc(e.LogFunc).
		Export("log")

	// get_header(name) -> string
	builder.NewFunctionBuilder().
		WithFunc(e.GetHeaderFunc).
		Export("get_header")

	// set_header(name, value) -> void
	builder.NewFunctionBuilder().
		WithFunc(e.SetHeaderFunc).
		Export("set_header")

	// get_body() -> bytes
	builder.NewFunctionBuilder().
		WithFunc(e.GetBodyFunc).
		Export("get_body")

	// set_body(body) -> void
	builder.NewFunctionBuilder().
		WithFunc(e.SetBodyFunc).
		Export("set_body")

	// get_metadata(key) -> string
	builder.NewFunctionBuilder().
		WithFunc(e.GetMetadataFunc).
		Export("get_metadata")

	// set_metadata(key, value) -> void
	builder.NewFunctionBuilder().
		WithFunc(e.SetMetadataFunc).
		Export("set_metadata")

	// get_tenant() -> string
	builder.NewFunctionBuilder().
		WithFunc(e.GetTenantFunc).
		Export("get_tenant")

	// get_api_key() -> string
	builder.NewFunctionBuilder().
		WithFunc(e.GetAPIKeyFunc).
		Export("get_api_key")

	// validate_api_key(api_key) -> bool
	builder.NewFunctionBuilder().
		WithFunc(e.ValidateAPIKeyFunc).
		Export("validate_api_key")

	// rate_limit(tenant_id, limit) -> bool
	builder.NewFunctionBuilder().
		WithFunc(e.RateLimitFunc).
		Export("rate_limit")

	return builder.Instantiate(ctx)
}

// ContextWithPluginInput creates a context with plugin input.
func ContextWithPluginInput(ctx context.Context, input *PluginInput) context.Context {
	return context.WithValue(ctx, "pluginInput", input)
}

// ContextWithPluginOutput creates a context with plugin output.
func ContextWithPluginOutput(ctx context.Context, output *PluginOutput) context.Context {
	return context.WithValue(ctx, "pluginOutput", output)
}

// JSONMarshal serializes data to JSON.
func JSONMarshal(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}

// JSONUnmarshal deserializes JSON to data.
func JSONUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
