package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"

	"github.com/wieghx/gateway-a/internal/metrics"
	"github.com/wieghx/gateway-a/internal/tokenizer"
	"github.com/wieghx/gateway-a/middleware/circuitbreaker"
)

// LLMHandler handles LLM streaming requests
type LLMHandler struct {
	upstreamURL string
	defaultKey  string
	// sharedCircuit is the single circuit breaker for the upstream (critical for stability)
	sharedCircuit *circuitbreaker.CircuitBreaker
	metrics       *metrics.Metrics // optional, for observability

	// Configurable per-handler
	maxRetries     int
	requestTimeout time.Duration
}

// NewLLMHandler creates a new LLM handler with a shared circuit breaker for the upstream.
func NewLLMHandler(upstreamURL, defaultKey string, m *metrics.Metrics, maxRetries int, timeout time.Duration) *LLMHandler {
	if upstreamURL == "" {
		upstreamURL = "https://api.openai.com/v1"
	}
	if maxRetries <= 0 {
		maxRetries = 2
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}

	cbCfg := circuitbreaker.DefaultCircuitBreakerConfig()
	cbCfg.FailureThreshold = 50.0
	cbCfg.ResetTimeout = 30 * time.Second

	return &LLMHandler{
		upstreamURL:    upstreamURL,
		defaultKey:     defaultKey,
		sharedCircuit:  circuitbreaker.NewCircuitBreaker(cbCfg),
		metrics:        m,
		maxRetries:     maxRetries,
		requestTimeout: timeout,
	}
}

// ChatCompletion handles OpenAI-compatible chat completion streaming
// POST /v1/chat/completions
func (h *LLMHandler) ChatCompletion(c context.Context, ctx *app.RequestContext) {
	// Get tenant from API key
	authHeader := ctx.Request.Header.Get("Authorization")
	if authHeader == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "Missing API key"})
		return
	}

	// Extract API key (remove "Bearer " prefix)
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")

	tenant, err := getTenantByAPIKey(apiKey)
	if err != nil {
		log.Printf("Invalid API key: %v", err)
		ctx.JSON(consts.StatusUnauthorized, map[string]string{"error": "Invalid API key"})
		return
	}

	// Read request body
	rawBody := ctx.Request.BodyBytes()

	// Basic protection: limit request body size (configurable in future)
	const maxBodySize = 1 << 20 // 1MB
	if len(rawBody) > maxBodySize {
		ctx.JSON(consts.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
		return
	}

	var req struct {
		Model     string          `json:"model"`
		Messages  json.RawMessage `json:"messages"`
		Stream    bool            `json:"stream"`
		MaxTokens int             `json:"max_tokens"`
	}
	if err := sonic.Unmarshal(rawBody, &req); err != nil {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "Invalid request format"})
		return
	}

	// Basic structural validation
	if len(req.Messages) == 0 {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "messages cannot be empty"})
		return
	}
	if req.Model == "" {
		ctx.JSON(consts.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}

	// Best-effort pre-flight quota check.
	// Real enforcement happens via post-response recording + cumulative checks on future requests.
	// Using rough estimate (messages * ~200 tokens) until we add a tokenizer.
	estimatedTokens := int64(len(req.Messages) * 200)
	if err := checkQuota(tenant.ID, estimatedTokens); err != nil {
		ctx.JSON(consts.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	// Forward to upstream LLM (use per-request key for passthrough or default)
	keyToUse := apiKey
	if keyToUse == "" {
		keyToUse = h.defaultKey
	}

	if h.metrics != nil {
		h.metrics.LLMRequest(req.Model, "openai", "started")
	}

	if req.Stream {
		h.streamChatCompletion(c, ctx, req, keyToUse, tenant.ID)
	} else {
		h.nonStreamChatCompletion(c, ctx, req, keyToUse, tenant.ID)
	}
}

// streamChatCompletion handles streaming response
func (h *LLMHandler) streamChatCompletion(c context.Context, ctx *app.RequestContext, req struct {
	Model     string          `json:"model"`
	Messages  json.RawMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens"`
}, apiKey string, tenantID uint) {
	start := time.Now()

	// Set streaming response headers
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")

	// Collect safe headers to forward
	extraHeaders := map[string]string{}
	if ua := ctx.Request.Header.Get("User-Agent"); ua != "" {
		extraHeaders["User-Agent"] = ua
	}
	if rid := ctx.Request.Header.Get("X-Request-ID"); rid != "" {
		extraHeaders["X-Request-ID"] = rid
	}
	if xff := ctx.Request.Header.Get("X-Forwarded-For"); xff != "" {
		extraHeaders["X-Forwarded-For"] = xff
	}

	// Create upstream request with real target (shared circuit breaker + light retry)
	upstreamClient := NewLLMClient(LLMClientConfig{
		BaseURL:      h.upstreamURL,
		APIKey:       apiKey,
		Circuit:      h.sharedCircuit,
		MaxRetries:   h.maxRetries,
		Timeout:      h.requestTimeout,
		ExtraHeaders: extraHeaders,
	})
	upstreamReq := &ChatCompletionRequest{
		Model:     req.Model,
		Messages:  parseMessages(req.Messages),
		Stream:    true,
		MaxTokens: &req.MaxTokens,
	}

	// Accumulate raw SSE data for post-stream usage extraction.
	// This enables real token accounting even for streaming responses when the
	// upstream includes usage (via stream_options.include_usage).
	var streamBuffer bytes.Buffer

	// Handle streaming - forward chunks to client while buffering for usage parsing
	err := upstreamClient.StreamChatCompletion(c, upstreamReq, func(chunk []byte) {
		// Forward to client
		ctx.Write([]byte("data: "))
		ctx.Write(chunk)
		ctx.Write([]byte("\n\n"))
		ctx.Flush()

		// Buffer raw chunk for later usage extraction
		streamBuffer.Write(chunk)
		streamBuffer.Write([]byte("\n"))
	})

	if err != nil {
		log.Printf("Stream error: %v", err)
		// Headers already sent for SSE; write error event instead of JSON
		ctx.Write([]byte("data: {\"error\": \"stream failed\"}\n\n"))
		ctx.Flush()
		return
	}

	// Log usage (redact key)
	zap.L().Info("LLM completion",
		zap.String("model", req.Model),
	)

	// Attempt to extract real usage from the streamed response
	extractedUsage := extractUsageFromStreamBuffer(streamBuffer.Bytes())

	if extractedUsage.TotalTokens == 0 {
		// Fallback: use the improved tokenizer package (model-aware heuristics)
		inputText := string(req.Messages)
		outputText := streamBuffer.String()

		cfg := tokenizer.DefaultConfig()

		estimated := Usage{
			PromptTokens:     tokenizer.EstimatePromptTokens(inputText, req.Model, cfg),
			CompletionTokens: tokenizer.EstimateCompletionTokens(outputText, req.Model, cfg),
			TotalTokens:      tokenizer.EstimatePromptTokens(inputText, req.Model, cfg) + tokenizer.EstimateCompletionTokens(outputText, req.Model, cfg),
		}
		recordUsage(tenantID, req.Model, estimated)
	} else {
		recordUsage(tenantID, req.Model, extractedUsage)
	}

	// Record latency and request completion for streaming
	latency := time.Since(start).Seconds()
	if h.metrics != nil {
		h.metrics.LLMRequest(req.Model, "openai", "success")
		h.metrics.LLMLatency(req.Model, "openai", latency)
		// Note: Token metrics for streaming are best-effort via recordUsage + extraction
	}

	// Note on streaming + quota:
	// Quota headers cannot be reliably set for streaming responses (headers must be sent before body).
	// Quota state is recorded after the stream completes. Enforcement happens on subsequent requests.
	// This is standard behavior for streaming LLM gateways.
}

// nonStreamChatCompletion handles non-streaming response
func (h *LLMHandler) nonStreamChatCompletion(c context.Context, ctx *app.RequestContext, req struct {
	Model     string          `json:"model"`
	Messages  json.RawMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens"`
}, apiKey string, tenantID uint) {
	start := time.Now()

	// Collect safe headers to forward
	extraHeaders := map[string]string{}
	if ua := ctx.Request.Header.Get("User-Agent"); ua != "" {
		extraHeaders["User-Agent"] = ua
	}
	if rid := ctx.Request.Header.Get("X-Request-ID"); rid != "" {
		extraHeaders["X-Request-ID"] = rid
	}
	if xff := ctx.Request.Header.Get("X-Forwarded-For"); xff != "" {
		extraHeaders["X-Forwarded-For"] = xff
	}

	// Create upstream request with real target (shared circuit breaker + light retry)
	upstreamClient := NewLLMClient(LLMClientConfig{
		BaseURL:      h.upstreamURL,
		APIKey:       apiKey,
		Circuit:      h.sharedCircuit,
		MaxRetries:   h.maxRetries,
		Timeout:      h.requestTimeout,
		ExtraHeaders: extraHeaders,
	})
	maxTokens := req.MaxTokens
	upstreamReq := &ChatCompletionRequest{
		Model:     req.Model,
		Messages:  parseMessages(req.Messages),
		Stream:    false,
		MaxTokens: &maxTokens,
	}

	resp, err := upstreamClient.ChatCompletion(c, upstreamReq)
	latency := time.Since(start).Seconds()

	if err != nil {
		if h.metrics != nil {
			h.metrics.LLMErorr(req.Model, "upstream_error")
		}
		if ue, ok := err.(*UpstreamError); ok {
			ctx.JSON(ue.StatusCode, map[string]string{"error": ue.Message})
			return
		}
		ctx.JSON(consts.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// Record real token usage (foundation for reliable quota)
	if resp != nil {
		recordUsage(tenantID, req.Model, resp.Usage)

		// Add quota observability headers
		used, remaining, limit := getQuotaStatus(tenantID)
		ctx.Response.Header.Set("X-Quota-Limit", fmt.Sprintf("%d", limit))
		ctx.Response.Header.Set("X-Quota-Used", fmt.Sprintf("%d", used))
		ctx.Response.Header.Set("X-Quota-Remaining", fmt.Sprintf("%d", remaining))

		// Record metrics
		if h.metrics != nil {
			h.metrics.LLMRequest(req.Model, "openai", "success")
			h.metrics.LLMTokens(req.Model, "prompt", resp.Usage.PromptTokens)
			h.metrics.LLMTokens(req.Model, "completion", resp.Usage.CompletionTokens)
			h.metrics.LLMLatency(req.Model, "openai", latency)
		}
	}

	ctx.JSON(consts.StatusOK, resp)
}

// parseMessages converts json.RawMessage to Message slice
func parseMessages(msg json.RawMessage) []Message {
	var msgs []Message
	if err := sonic.Unmarshal(msg, &msgs); err != nil {
		return []Message{}
	}
	return msgs
}

// HealthCheck handles health check endpoint
func (h *LLMHandler) HealthCheck(c context.Context, ctx *app.RequestContext) {
	ctx.JSON(consts.StatusOK, map[string]string{
		"status":  "ok",
		"service": "gateway-a-llm",
	})
}
