# Gateway-A Plugin Development Guide

## Overview

This guide covers plugin development for Gateway-A. Plugins allow you to extend the gateway's functionality without modifying the core codebase.

**Version:** 1.0.0  
**Last Updated:** 2026-05-12

---

## Table of Contents

1. [Introduction](#introduction)
2. [Plugin Architecture](#plugin-architecture)
3. [Plugin SDK](#plugin-sdk)
4. [Developing a Plugin](#developing-a-plugin)
5. [Example Plugins](#example-plugins)
6. [Plugin Lifecycle](#plugin-lifecycle)
7. [Testing Plugins](#testing-plugins)
8. [Packaging and Distribution](#packaging-and-distribution)
9. [Best Practices](#best-practices)

---

## Introduction

### What are Plugins?

Plugins are Go modules that extend Gateway-A's functionality. They can:

- Add custom rate limiting strategies
- Implement custom circuit breakers
- Add authentication providers
- Create custom middleware
- Extend queue job handlers
- Add custom logging formats

### Plugin Types

| Type | Description | Use Case |
|------|-------------|----------|
| **Middleware** | HTTP request/response middleware | Authentication, custom headers |
| **Rate Limiter** | Custom rate limiting algorithms | API quotas, resource limits |
| **Circuit Breaker** | Custom circuit breaker implementations | Service protection |
| **Job Handler** | Background task processors | Token calculation, caching |
| **Auth Provider** | Authentication backends | OAuth, API key, JWT |

---

## Plugin Architecture

### Plugin Interface

All plugins must implement the `Plugin` interface:

```go
type Plugin interface {
    Name() string
    Version() string
    Description() string
    Init(config Config) error
    Start() error
    Stop() error
}
```

### Plugin Structure

```
my-plugin/
├── go.mod
├── main.go          # Plugin entry point
├── plugin.go        # Plugin implementation
├── config.go        # Configuration types
├── handlers/        # Handler implementations
├── middleware/      # Middleware implementations
└── README.md
```

---

## Plugin SDK

### Basic Plugin Template

```go
package myplugin

import (
    "context"
    "fmt"
)

// Config represents plugin configuration
type Config struct {
    Name      string `mapstructure:"name"`
    Enabled   bool   `mapstructure:"enabled"`
    Timeout   int    `mapstructure:"timeout"`
    APIKey    string `mapstructure:"api_key"`
}

// Plugin implements the Plugin interface
type Plugin struct {
    config Config
    stopped bool
}

// Name returns the plugin name
func (p *Plugin) Name() string {
    return "my-plugin"
}

// Version returns the plugin version
func (p *Plugin) Version() string {
    return "1.0.0"
}

// Description returns the plugin description
func (p *Plugin) Description() string {
    return "My custom plugin for Gateway-A"
}

// Init initializes the plugin with configuration
func (p *Plugin) Init(config Config) error {
    p.config = config
    if !p.config.Enabled {
        return fmt.Errorf("plugin is disabled")
    }
    return nil
}

// Start starts the plugin
func (p *Plugin) Start() error {
    if p.stopped {
        return fmt.Errorf("plugin has been stopped")
    }
    // Initialize resources, connections, etc.
    return nil
}

// Stop stops the plugin
func (p *Plugin) Stop() error {
    p.stopped = true
    // Clean up resources
    return nil
}
```

---

## Developing a Plugin

### Step 1: Create Plugin Module

```bash
# Create plugin directory
mkdir -p ~/plugins/custom-auth
cd ~/plugins/custom-auth

# Initialize Go module
go mod init github.com/your-org/custom-auth

# Create go.mod
cat > go.mod << EOF
module github.com/your-org/custom-auth

go 1.21

require github.com/gu/gateway-a v0.1.0

require (
    github.com/cloudwego/hertz v0.8.0
    go.uber.org/zap v1.26.0
)
EOF
```

### Step 2: Implement Plugin

```go
package main

import (
    "context"
    "fmt"
    "github.com/cloudwego/hertz/pkg/app"
    "github.com/gu/gateway-a/wasm"
)

// Config holds plugin configuration
type Config struct {
    Enabled     bool   `mapstructure:"enabled"`
    APIKey      string `mapstructure:"api_key"`
    AllowPaths  []string `mapstructure:"allowed_paths"`
}

// CustomAuthPlugin is a custom authentication plugin
type CustomAuthPlugin struct {
    config Config
}

// Name returns the plugin name
func (p *CustomAuthPlugin) Name() string {
    return "custom-auth"
}

// Version returns the plugin version
func (p *CustomAuthPlugin) Version() string {
    return "1.0.0"
}

// Description returns the plugin description
func (p *CustomAuthPlugin) Description() string {
    return "Custom authentication plugin with API key and path-based access"
}

// Init initializes the plugin
func (p *CustomAuthPlugin) Init(config Config) error {
    p.config = config
    return nil
}

// Start starts the plugin
func (p *CustomAuthPlugin) Start() error {
    return nil
}

// Stop stops the plugin
func (p *CustomAuthPlugin) Stop() error {
    return nil
}

// Middleware returns the middleware handler
func (p *CustomAuthPlugin) Middleware() app.HandlerFunc {
    return func(c context.Context, ctx *app.RequestContext) {
        // Skip allowed paths
        for _, path := range p.config.AllowPaths {
            if ctx.Request.URI().Path() == path {
                ctx.Next(c)
                return
            }
        }

        // Check API key
        authHeader := ctx.Request.Header.Get("Authorization")
        if authHeader != "Bearer "+p.config.APIKey {
            ctx.JSON(401, map[string]string{
                "error": "Unauthorized",
            })
            return
        }

        ctx.Next(c)
    }
}

// Register registers the plugin with Gateway-A
func Register() {
    wasm.RegisterPlugin(&CustomAuthPlugin{})
}
```

### Step 3: Build Plugin

```bash
# Build the plugin
go build -o custom-auth.so -buildmode=plugin main.go
```

---

## Example Plugins

### Example 1: Custom Rate Limiter

```go
package ratelimit

import (
    "context"
    "time"
    "github.com/gu/gateway-a/middleware/ratelimit"
)

// SlidingWindowLimiter implements a sliding window rate limiter
type SlidingWindowLimiter struct {
    windowSize time.Duration
    maxRequests int
    client *redis.Client
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter
func NewSlidingWindowLimiter(window time.Duration, maxRequests int) *SlidingWindowLimiter {
    return &SlidingWindowLimiter{
        windowSize: window,
        maxRequests: maxRequests,
    }
}

// Check checks if request is allowed
func (l *SlidingWindowLimiter) Check(ctx context.Context, key string) (*ratelimit.RateLimitResult, error) {
    now := time.Now()
    windowStart := now.Add(-l.windowSize)

    // Count requests in window
    count := l.client.ZCount(ctx, "ratelimit:"+key, windowStart.UnixMilli(), now.UnixMilli()).Val()

    if count >= int64(l.maxRequests) {
        return &ratelimit.RateLimitResult{
            Allowed:     false,
            Limit:       int64(l.maxRequests),
            Remaining:   0,
            ResetAt:     windowStart,
        }, nil
    }

    // Add current request
    l.client.ZAdd(ctx, "ratelimit:"+key, &redis.Z{
        Score:  now.UnixMilli(),
        Member:  now.String(),
    })

    return &ratelimit.RateLimitResult{
        Allowed:     true,
        Limit:       int64(l.maxRequests),
        Remaining:   int64(l.maxRequests - count - 1),
        ResetAt:     windowStart,
    }, nil
}
```

### Example 2: Custom Circuit Breaker

```go
package circuitbreaker

import (
    "context"
    "errors"
    "github.com/gu/gateway-a/middleware/circuitbreaker"
)

// RetryCircuitBreaker implements a circuit breaker with retry logic
type RetryCircuitBreaker struct {
    baseBreaker *circuitbreaker.CircuitBreaker
    maxRetries  int
    retryDelay  time.Duration
}

// NewRetryCircuitBreaker creates a new retry circuit breaker
func NewRetryCircuitBreaker(config circuitbreaker.CircuitBreakerConfig, maxRetries int, retryDelay time.Duration) *RetryCircuitBreaker {
    return &RetryCircuitBreaker{
        baseBreaker: circuitbreaker.NewCircuitBreaker(config),
        maxRetries: maxRetries,
        retryDelay: retryDelay,
    }
}

// Call executes with retry logic
func (cb *RetryCircuitBreaker) Call(ctx context.Context, fn func() error) error {
    var lastErr error

    for i := 0; i <= cb.maxRetries; i++ {
        err := cb.baseBreaker.Call(ctx, fn)
        if err == nil {
            return nil
        }

        lastErr = err

        if i < cb.maxRetries {
            select {
            case <-time.After(cb.retryDelay):
                continue
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }

    return lastErr
}
```

### Example 3: Queue Job Handler

```go
package queue

import (
    "context"
    "fmt"
    "github.com/gu/gateway-a/pkg/queue"
)

// EmailNotificationJob handles email notifications
type EmailNotificationJob struct {
    queueHandler queue.JobHandler
    smtpHost     string
    smtpPort     int
}

// NewEmailNotificationJob creates a new email notification handler
func NewEmailNotificationJob(smtpHost string, smtpPort int) *EmailNotificationJob {
    return &EmailNotificationJob{
        smtpHost: smtpHost,
        smtpPort: smtpPort,
    }
}

// Handle processes the email notification job
func (h *EmailNotificationJob) Handle(job *queue.Job) error {
    var payload struct {
        To      string   `json:"to"`
        Subject string   `json:"subject"`
        Body    string   `json:"body"`
    }

    if err := json.Unmarshal(job.Payload, &payload); err != nil {
        return fmt.Errorf("failed to parse payload: %w", err)
    }

    // Send email
    if err := sendEmail(h.smtpHost, h.smtpPort, payload.To, payload.Subject, payload.Body); err != nil {
        return fmt.Errorf("failed to send email: %w", err)
    }

    return nil
}

func sendEmail(host string, port int, to, subject, body string) error {
    // Implement email sending logic
    return nil
}
```

---

## Plugin Lifecycle

### Plugin Initialization Order

1. **Load** - Plugin is loaded from file
2. **Init** - Configuration is applied
3. **Start** - Plugin resources are initialized
4. **Ready** - Plugin is ready to handle requests
5. **Stop** - Plugin resources are cleaned up

### Lifecycle Hooks

```go
type PluginLifecycle interface {
    OnLoad() error              // Called after loading
    OnInit(config Config) error // Called after initialization
    OnStart() error             // Called when starting
    OnStop() error              // Called when stopping
    OnReady() error             // Called when plugin is ready
}
```

### Example Lifecycle Implementation

```go
type MyPlugin struct {
    config Config
    ready  bool
}

func (p *MyPlugin) OnLoad() error {
    log.Println("MyPlugin: Loading...")
    return nil
}

func (p *MyPlugin) OnInit(config Config) error {
    log.Println("MyPlugin: Initializing with config:", config)
    p.config = config
    return nil
}

func (p *MyPlugin) OnStart() error {
    log.Println("MyPlugin: Starting...")
    return nil
}

func (p *MyPlugin) OnStop() error {
    log.Println("MyPlugin: Stopping...")
    return nil
}

func (p *MyPlugin) OnReady() error {
    p.ready = true
    log.Println("MyPlugin: Ready")
    return nil
}
```

---

## Testing Plugins

### Unit Testing

```go
package myplugin

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
    config := Config{
        Name:     "test",
        Enabled:  true,
        APIKey:   "test-key",
    }

    plugin := &CustomAuthPlugin{}
    err := plugin.Init(config)

    assert.NoError(t, err)
    assert.Equal(t, "test", plugin.config.Name)
}

func TestMiddleware(t *testing.T) {
    config := Config{
        Enabled:    true,
        APIKey:     "test-key",
        AllowPaths: []string{"/health"},
    }

    plugin := &CustomAuthPlugin{}
    err := plugin.Init(config)
    assert.NoError(t, err)

    middleware := plugin.Middleware()
    assert.NotNil(t, middleware)
}
```

### Integration Testing

```go
package myplugin

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestMiddlewareIntegration(t *testing.T) {
    config := Config{
        Enabled:  true,
        APIKey:   "test-key",
        AllowPaths: []string{"/health"},
    }

    plugin := &CustomAuthPlugin{}
    plugin.Init(config)

    middleware := plugin.Middleware()

    // Test allowed path
    req := httptest.NewRequest("GET", "/health", nil)
    w := httptest.NewRecorder()
    ctx := context.Background()

    middleware(ctx, &app.RequestContext{Request: req, Response: &protocol.Response{}})
    assert.Equal(t, http.StatusNoContent, w.Code)
}
```

### Test Plugin Container

```go
package integration

import (
    "testing"
    "github.com/stretchr/testify/suite"
)

type PluginTestSuite struct {
    suite.Suite
    plugin *CustomAuthPlugin
    server *testcontainers.Container
}

func (s *PluginTestSuite) SetupSuite() {
    s.plugin = &CustomAuthPlugin{}
    s.plugin.Init(Config{
        Enabled:  true,
        APIKey:   "test-key",
    })
}

func (s *PluginTestSuite) TestPluginInitialization() {
    err := s.plugin.Start()
    s.Require().NoError(err)
    s.plugin.Stop()
}
```

---

## Packaging and Distribution

### Plugin Structure

```
my-plugin/
├── go.mod
├── go.sum
├── main.go
├── plugin.go
├── config.go
├── README.md
└── dist/
    ├── my-plugin.so          # Compiled plugin
    ├── config.yaml.example   # Example configuration
    └── LICENSE
```

### Building Plugin

```bash
# Build shared library
go build -buildmode=plugin -o dist/my-plugin.so .

# Create config example
cat > dist/config.yaml.example << EOF
name: my-plugin
version: 1.0.0
enabled: true
api_key: your-api-key
allowed_paths:
  - /health
  - /metrics
EOF
```

### Plugin Manifest

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Custom authentication plugin",
  "type": "middleware",
  "author": "Your Company",
  "license": "MIT",
  "gateway_version": ">=0.1.0",
  "config_schema": {
    "name": "string",
    "enabled": "boolean",
    "api_key": "string",
    "allowed_paths": "array"
  }
}
```

### Installing Plugin

```bash
# Copy plugin to plugins directory
cp dist/my-plugin.so /path/to/gateway/plugins/

# Configure in config.yaml
cat > config.yaml << EOF
plugins:
  - name: my-plugin
    path: /path/to/gateway/plugins/my-plugin.so
    enabled: true
    config:
      name: my-plugin
      api_key: your-api-key
      allowed_paths:
        - /health
        - /metrics
EOF
```

---

## Best Practices

### Security

1. **Validate all input** - Never trust plugin input
2. **Use context cancellation** - Respect request timeouts
3. **Avoid sensitive data** - Don't log API keys or tokens
4. **Validate configuration** - Ensure config values are valid
5. **Use least privilege** - Only access necessary resources

### Performance

1. **Cache results** - Store frequently accessed data
2. **Use connection pooling** - Reuse database/HTTP connections
3. **Async processing** - Use goroutines for non-blocking operations
4. **Avoid blocking calls** - Use timeouts and context
5. **Profile regularly** - Use pprof to identify bottlenecks

### Code Quality

1. **Document public API** - Use godoc comments
2. **Write tests** - Unit and integration tests
3. **Follow Go standards** - Use gofmt, go vet, golint
4. **Handle errors** - Don't ignore errors
5. **Clean up resources** - Implement proper Stop()

### Configuration

1. **Use sensible defaults** - Make plugins work out of the box
2. **Make config file-based** - Support YAML/JSON config
3. **Validate config** - Check required fields
4. **Support env vars** - Allow environment variable overrides
5. **Document config** - Explain each config option

---

## Plugin Registration

### Registering Plugin

```go
// main.go
package main

import (
    "github.com/gu/gateway-a/wasm"
    "github.com/your-org/my-plugin"
)

func main() {
    // Register plugins
    wasm.RegisterPlugin(&myplugin.CustomAuthPlugin{})
    wasm.RegisterPlugin(&ratelimit.SlidingWindowLimiter{})

    // Start gateway
    gateway.Run()
}
```

### Plugin Discovery

```go
// wasm/plugin.go
func RegisterPlugin(p Plugin) {
    plugins = append(plugins, p)
}

func GetPlugins() []Plugin {
    return plugins
}

func LoadPlugins(path string) error {
    // Load plugins from directory
    files, _ := filepath.Glob(path + "/*.so")
    for _, file := range files {
        plugin, err := loadPlugin(file)
        if err != nil {
            return err
        }
        RegisterPlugin(plugin)
    }
    return nil
}
```

---

## Debugging

### Enable Debug Mode

```yaml
plugins:
  - name: my-plugin
    debug: true
    log_level: debug
```

### Plugin Logs

```bash
# Check plugin logs
kubectl logs -f deployment/gateway-a -n gateway-a | grep my-plugin

# Enable verbose logging
kubectl set env deployment/gateway-a LOG_LEVEL=debug -n gateway-a
```

### Common Debug Commands

```bash
# Check loaded plugins
curl http://localhost:8080/health/plugins

# List available plugins
kubectl exec -it gateway-a-<pod> -- ls /plugins

# Test plugin config
cat config.yaml | grep -A 10 "plugins:"
```

---

## Troubleshooting

### Plugin Won't Load

1. **Check file permissions**
   ```bash
   chmod 644 /path/to/plugin.so
   ```

2. **Verify Go version**
   ```bash
   go version  # Must match gateway Go version
   ```

3. **Check dependencies**
   ```bash
   go mod verify
   ```

### Plugin Causes Panic

1. **Enable crash protection**
   ```go
   defer func() {
       if r := recover(); r != nil {
           log.Printf("Plugin panic: %v", r)
       }
   }()
   ```

2. **Use panic recovery**
   ```go
   func safeExecute(fn func() error) {
       defer func() {
           if r := recover(); r != nil {
               fmt.Printf("Recovered from panic: %v\n", r)
           }
       }()
       fn()
   }
   ```

### Configuration Errors

1. **Validate config schema**
   ```go
   func (p *Plugin) ValidateConfig(config Config) error {
       if config.APIKey == "" {
           return errors.New("api_key is required")
       }
       return nil
   }
   ```

---

## Resources

- [Go Plugins](https://pkg.go.dev/cmd/go#hdr-Compile_and_test_go_packages_with_the_plugin_flag)
- [Gateway-A API Docs](https://gateway-a.example.com/docs)
- [GitHub Repository](https://github.com/gateway-a/plugins)

---

## Changelog

- **v1.0.0** (2026-05-12) - Initial plugin development guide
