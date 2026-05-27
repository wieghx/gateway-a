package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/wieghx/gateway-a/middleware/circuitbreaker"
)

// LLMClientConfig holds configuration for the LLM client
type LLMClientConfig struct {
	BaseURL    string
	APIKey     string
	Circuit    *circuitbreaker.CircuitBreaker // shared circuit breaker - must be provided
	HTTPClient *http.Client                   // optional, will create default if nil

	// MaxRetries controls how many times we retry on transient upstream errors (5xx + network).
	// Default is 1 (no extra retry). Recommended: 2 or 3 for production.
	MaxRetries int

	// ExtraHeaders allows forwarding selected headers from the original client request
	ExtraHeaders map[string]string

	// Timeout overrides the default HTTP client timeout for this client instance
	Timeout time.Duration
}

// LLMClient handles proxying to upstream LLM providers
type LLMClient struct {
	baseURL      string
	apiKey       string
	circuit      *circuitbreaker.CircuitBreaker
	httpClient   *http.Client
	maxRetries   int
	extraHeaders map[string]string
}

// NewLLMClient creates a new LLM client using the provided config.
// The CircuitBreaker should be shared across requests for proper state tracking.
func NewLLMClient(cfg LLMClientConfig) *LLMClient {
	if cfg.Circuit == nil {
		// Fallback (not recommended for production) - creates isolated breaker
		cbCfg := circuitbreaker.DefaultCircuitBreakerConfig()
		cfg.Circuit = circuitbreaker.NewCircuitBreaker(cbCfg)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		httpClient = &http.Client{
			Timeout: timeout,
		}
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 1 {
		maxRetries = 1 // at least one attempt
	}

	client := &LLMClient{
		baseURL:      cfg.BaseURL,
		apiKey:       cfg.APIKey,
		circuit:      cfg.Circuit,
		httpClient:   httpClient,
		maxRetries:   maxRetries,
		extraHeaders: cfg.ExtraHeaders,
	}

	return client
}

// StreamOptions controls streaming behavior (OpenAI compatible)
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatCompletionRequest represents an OpenAI-compatible chat completion request
type ChatCompletionRequest struct {
	Model         string           `json:"model"`
	Messages      []Message        `json:"messages"`
	Stream        bool             `json:"stream"`
	StreamOptions *StreamOptions   `json:"stream_options,omitempty"`
	MaxTokens     *int             `json:"max_tokens,omitempty"`
	Temperature   *float64         `json:"temperature,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse represents an OpenAI-compatible chat completion response
type ChatCompletionResponse struct {
	ID              string           `json:"id"`
	Object          string           `json:"object"`
	Created         int64            `json:"created"`
	Model           string           `json:"model"`
	Choices         []Choice         `json:"choices"`
	Usage           Usage            `json:"usage"`
	SystemFingerprint string         `json:"system_fingerprint"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int              `json:"index"`
	Message      Message          `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// ChatCompletion sends a chat completion request to the upstream LLM
func (c *LLMClient) ChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	var result *ChatCompletionResponse

	err := c.circuit.Call(ctx, func() error {
		var lastErr error

		for attempt := 0; attempt < c.maxRetries; attempt++ {
			if attempt > 0 {
				// Simple exponential backoff
				backoff := time.Duration(100<<uint(attempt-1)) * time.Millisecond
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			data, err := sonic.Marshal(req)
			if err != nil {
				return fmt.Errorf("failed to marshal request: %w", err)
			}

			url := fmt.Sprintf("%s/chat/completions", c.baseURL)
			httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
			httpReq.Header.Set("Content-Type", "application/json")

			// Apply extra headers (safe forwarding from original request)
			for k, v := range c.extraHeaders {
				if httpReq.Header.Get(k) == "" {
					httpReq.Header.Set(k, v)
				}
			}

			resp, err := c.httpClient.Do(httpReq)
			if err != nil {
				lastErr = fmt.Errorf("failed to send request: %w", err)
				if isTransientError(lastErr) && attempt < c.maxRetries-1 {
					continue
				}
				return lastErr
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				lastErr = fmt.Errorf("upstream error: %s - %s", resp.Status, string(body))

				if resp.StatusCode >= 400 && resp.StatusCode < 500 {
					return &UpstreamError{StatusCode: resp.StatusCode, Message: string(body)}
				}
				if isRetryableStatusCode(resp.StatusCode) && attempt < c.maxRetries-1 {
					continue
				}
				return lastErr
			}

			var chatResp ChatCompletionResponse
			if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			result = &chatResp
			return nil
		}

		return lastErr
	})

	if err != nil {
		return nil, fmt.Errorf("circuit breaker error: %w", err)
	}

	return result, nil
}

// StreamChatCompletion sends a streaming chat completion request
func (c *LLMClient) StreamChatCompletion(ctx context.Context, req *ChatCompletionRequest, handler func(chunk []byte)) error {
	var err error

	err = c.circuit.Call(ctx, func() error {
		// Automatically request usage information for better token accounting
		streamReq := *req // shallow copy of struct
		if streamReq.Stream {
			if streamReq.StreamOptions == nil {
				streamReq.StreamOptions = &StreamOptions{}
			}
			streamReq.StreamOptions.IncludeUsage = true
		}

		data, err := sonic.Marshal(&streamReq)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		url := fmt.Sprintf("%s/chat/completions", c.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		// Apply extra headers passed from caller (safe forwarding)
		for k, v := range c.extraHeaders {
			if httpReq.Header.Get(k) == "" {
				httpReq.Header.Set(k, v)
			}
		}

		// Forward a small set of useful headers
		if c.apiKey != "" {
			// already set above
		}
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("upstream error: %s - %s", resp.Status, string(body))
		}

		// Read streaming response
		buf := make([]byte, 1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return fmt.Errorf("failed to read response: %w", readErr)
			}

			chunk := buf[:n]
			if len(chunk) > 0 {
				handler(chunk)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("circuit breaker error: %w", err)
	}

	return nil
}

// isTransientError determines if an error from upstream should trigger a retry.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	// Simple heuristic: treat timeout and connection errors as transient.
	// In production this can be expanded.
	return strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "EOF")
}

// isRetryableStatusCode returns true for status codes we should retry.
func isRetryableStatusCode(status int) bool {
	return status >= 500 && status <= 599
}

// UpstreamError represents an error returned from the upstream LLM provider
// with the original status code preserved for better client responses.
type UpstreamError struct {
	StatusCode int
	Message    string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream %d: %s", e.StatusCode, e.Message)
}

