package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wieghx/gateway-a/middleware/circuitbreaker"
)

func TestUpstreamError(t *testing.T) {
	err := &UpstreamError{StatusCode: 429, Message: "rate limited"}
	assert.Equal(t, "upstream 429: rate limited", err.Error())
	// Type assertion check
	var _ *UpstreamError = err
}

func TestLLMClientConfig_Defaults(t *testing.T) {
	cfg := LLMClientConfig{}
	client := NewLLMClient(cfg)

	assert.NotNil(t, client)
	assert.Equal(t, 1, client.maxRetries) // default
	assert.NotNil(t, client.httpClient)
}

func TestLLMClient_RetriesOnTransientError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"total_tokens":10}}`))
	}))
	defer server.Close()

	cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.DefaultCircuitBreakerConfig())
	client := NewLLMClient(LLMClientConfig{
		BaseURL:    server.URL,
		Circuit:    cb,
		MaxRetries: 3,
	})

	req := &ChatCompletionRequest{Model: "test", Messages: []Message{{Role: "user", Content: "hi"}}}
	resp, err := client.ChatCompletion(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, int64(10), resp.Usage.TotalTokens)
	assert.GreaterOrEqual(t, attempts, 2)
}

func TestLLMClient_RespectsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.DefaultCircuitBreakerConfig())
	client := NewLLMClient(LLMClientConfig{
		BaseURL:    server.URL,
		Circuit:    cb,
		Timeout:    100 * time.Millisecond,
		MaxRetries: 1,
	})

	req := &ChatCompletionRequest{Model: "test", Messages: []Message{{Role: "user", Content: "hi"}}}
	_, err := client.ChatCompletion(context.Background(), req)

	assert.Error(t, err)
}
