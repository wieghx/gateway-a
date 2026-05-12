package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gu/gateway-a/middleware/circuitbreaker"
)

// LLMClient handles proxying to upstream LLM providers
type LLMClient struct {
	baseURL  string
	apiKey   string
	circuit  *circuitbreaker.CircuitBreaker
	httpClient *http.Client
}

// NewLLMClient creates a new LLM client
func NewLLMClient(baseURL, apiKey string) *LLMClient {
	config := circuitbreaker.DefaultCircuitBreakerConfig()
	config.FailureThreshold = 50.0 // 50% failure rate
	config.ResetTimeout = 30 * time.Second

	cb := circuitbreaker.NewCircuitBreaker(config)

	return &LLMClient{
		baseURL:  baseURL,
		apiKey:   apiKey,
		circuit:  cb,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// ChatCompletionRequest represents an OpenAI-compatible chat completion request
type ChatCompletionRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Stream    bool             `json:"stream"`
	MaxTokens *int             `json:"max_tokens,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
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
	var err error

	// Use circuit breaker to wrap the request
	err = c.circuit.Call(ctx, func() error {
		data, err := json.Marshal(req)
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

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("upstream error: %s - %s", resp.Status, string(body))
		}

		var chatResp ChatCompletionResponse
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		result = &chatResp
		return nil
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
		data, err := json.Marshal(req)
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
