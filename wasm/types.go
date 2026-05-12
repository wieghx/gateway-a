// Package wasm provides the plugin SDK and runtime integration for the AI Gateway.
// It supports loading and executing .wasm plugins using WasmEdge/wazero.
package wasm

import (
	"context"
)

// PluginInput represents the input for a plugin hook.
// This is the data that flows from the gateway to the plugin.
type PluginInput struct {
	// Method is the HTTP method (GET, POST, PUT, DELETE, etc.)
	Method string `json:"method"`

	// URL is the full request URL
	URL string `json:"url"`

	// Headers contains the HTTP request headers
	Headers map[string]string `json:"headers"`

	// Body contains the request body bytes
	Body []byte `json:"body"`

	// TenantID is the identifier for the tenant making the request
	TenantID string `json:"tenant_id"`

	// APIKey is the API key used for authentication
	APIKey string `json:"api_key"`

	// Context holds any additional context passed through the plugin chain
	Context map[string]interface{} `json:"-"`
}

// PluginOutput represents the output from a plugin hook.
// This is the data that flows from the plugin back to the gateway.
type PluginOutput struct {
	// Status is the HTTP status code (e.g., 200, 404, 500)
	// For OnRequest, this can override the request with an early response
	Status int `json:"status"`

	// Headers contains the HTTP response headers
	Headers map[string]string `json:"headers"`

	// Body contains the response body bytes
	Body []byte `json:"body"`

	// Metadata contains arbitrary key-value pairs for plugin-specific data
	Metadata map[string]string `json:"metadata"`

	// Continue indicates whether the gateway should continue processing
	// Set to false to halt the request/response chain
	Continue bool `json:"continue"`
}

// ErrorInput represents input for the OnError hook.
type ErrorInput struct {
	// PluginInput is the original request input
	PluginInput *PluginInput `json:"input"`

	// ErrorCode is the error code
	ErrorCode string `json:"error_code"`

	// ErrorMessage is the error message
	ErrorMessage string `json:"error_message"`

	// WrappedErr is the underlying Go error (not serialized)
	WrappedErr error `json:"-"`
}

// ErrorOutput represents output from the OnError hook.
type ErrorOutput struct {
	// Status is the HTTP status code to return
	Status int `json:"status"`

	// Headers contains response headers
	Headers map[string]string `json:"headers"`

	// Body contains the error response body
	Body []byte `json:"body"`

	// Metadata contains additional error metadata
	Metadata map[string]string `json:"metadata"`
}

// NewPlugin creates a new Plugin instance.
func NewPlugin() *Plugin {
	return &Plugin{}
}

// Plugin defines the main plugin interface with lifecycle hooks.
type Plugin struct {
	// OnRequest is called before the request is forwarded to the upstream.
	// Returns a PluginOutput that can modify the request or short-circuit the flow.
	// If Continue is false in the output, the request chain is halted.
	OnRequest func(*PluginInput) (*PluginOutput, error)

	// OnResponse is called after receiving the response from the upstream.
	// Can modify the response or short-circuit the flow.
	OnResponse func(*PluginOutput) (*PluginOutput, error)

	// OnError is called when an error occurs during request processing.
	// Can provide custom error responses.
	OnError func(*ErrorInput) (*ErrorOutput, error)
}

// PluginFactory creates new plugin instances.
type PluginFactory interface {
	// New creates a new plugin instance
	New() *Plugin
}

// PluginChain represents a chain of plugins to be executed.
type PluginChain struct {
	plugins []*Plugin
}

// NewPluginChain creates a new plugin chain.
func NewPluginChain() *PluginChain {
	return &PluginChain{
		plugins: make([]*Plugin, 0),
	}
}

// Add adds a plugin to the chain.
func (c *PluginChain) Add(plugin *Plugin) *PluginChain {
	c.plugins = append(c.plugins, plugin)
	return c
}

// ExecuteOnRequest executes all OnRequest hooks in order.
func (c *PluginChain) ExecuteOnRequest(ctx context.Context, input *PluginInput) (*PluginOutput, error) {
	var output *PluginOutput
	var err error
	current := input

	for _, plugin := range c.plugins {
		if plugin.OnRequest == nil {
			continue
		}
		output, err = plugin.OnRequest(current)
		if err != nil {
			return nil, err
		}
		if output == nil {
			continue
		}
		// If Continue is false, halt the chain
		if !output.Continue {
			return output, nil
		}
		// Propagate metadata for next plugin in chain
		if current.Context == nil {
			current.Context = make(map[string]interface{})
		}
		if output.Metadata != nil {
			for k, v := range output.Metadata {
				current.Context[k] = v
			}
		}
	}
	return output, nil
}

// ExecuteOnResponse executes all OnResponse hooks in order.
func (c *PluginChain) ExecuteOnResponse(input *PluginOutput) (*PluginOutput, error) {
	var output *PluginOutput
	var err error
	current := input

	for _, plugin := range c.plugins {
		if plugin.OnResponse == nil {
			continue
		}
		output, err = plugin.OnResponse(current)
		if err != nil {
			return nil, err
		}
		if output == nil {
			continue
		}
		if !output.Continue {
			return output, nil
		}
		current = output
	}
	return current, nil
}

// ExecuteOnError executes the OnError hook.
func (c *PluginChain) ExecuteOnError(input *ErrorInput) (*ErrorOutput, error) {
	var output *ErrorOutput
	var err error

	for _, plugin := range c.plugins {
		if plugin.OnError == nil {
			continue
		}
		output, err = plugin.OnError(input)
		if err != nil {
			return nil, err
		}
		if output != nil {
			return output, nil
		}
	}
	return nil, nil
}

// PluginMetadata contains metadata about a loaded plugin.
type PluginMetadata struct {
	// Name is the plugin name
	Name string `json:"name"`

	// Version is the plugin version
	Version string `json:"version"`

	// Description is the plugin description
	Description string `json:"description"`

	// Hooks lists the available hooks (OnRequest, OnResponse, OnError)
	Hooks []string `json:"hooks"`

	// Author is the plugin author
	Author string `json:"author"`

	// Path is the file path of the plugin
	Path string `json:"path"`
}
