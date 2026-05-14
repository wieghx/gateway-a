package handler

import (
	"context"
	"github.com/bytedance/sonic"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// LLMHandler handles LLM streaming requests
type LLMHandler struct {
	proxyClient *http.Client
}

// NewLLMHandler creates a new LLM handler
func NewLLMHandler() *LLMHandler {
	return &LLMHandler{
		proxyClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
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

	// Check quota
	if err := checkQuota(tenant.ID, int64(len(req.Messages))); err != nil {
		ctx.JSON(consts.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}

	// Forward to upstream LLM
	if req.Stream {
		h.streamChatCompletion(c, ctx, req, apiKey, rawBody)
	} else {
		h.nonStreamChatCompletion(c, ctx, req, apiKey, rawBody)
	}
}

// streamChatCompletion handles streaming response
func (h *LLMHandler) streamChatCompletion(c context.Context, ctx *app.RequestContext, req struct {
	Model     string          `json:"model"`
	Messages  json.RawMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens"`
}, apiKey string, rawBody []byte) {
	// Set streaming response headers
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")

	// Create upstream request
	upstreamClient := NewLLMClient("", "")
	upstreamReq := &ChatCompletionRequest{
		Model:     req.Model,
		Messages:  parseMessages(req.Messages),
		Stream:    true,
		MaxTokens: &req.MaxTokens,
	}

	// Handle streaming
	err := upstreamClient.StreamChatCompletion(c, upstreamReq, func(chunk []byte) {
		// Write SSE format
		ctx.Write([]byte("data: "))
		ctx.Write(chunk)
		ctx.Write([]byte("\n\n"))
		ctx.Flush()
	})

	if err != nil {
		log.Printf("Stream error: %v", err)
		ctx.JSON(consts.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// Log usage
	zap.L().Info("LLM completion",
		zap.String("api_key", apiKey),
		zap.String("model", req.Model),
	)
}

// nonStreamChatCompletion handles non-streaming response
func (h *LLMHandler) nonStreamChatCompletion(c context.Context, ctx *app.RequestContext, req struct {
	Model     string          `json:"model"`
	Messages  json.RawMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens"`
}, apiKey string, rawBody []byte) {
	// Create upstream request
	upstreamClient := NewLLMClient("", "")
	maxTokens := req.MaxTokens
	upstreamReq := &ChatCompletionRequest{
		Model:     req.Model,
		Messages:  parseMessages(req.Messages),
		Stream:    false,
		MaxTokens: &maxTokens,
	}

	resp, err := upstreamClient.ChatCompletion(c, upstreamReq)
	if err != nil {
		ctx.JSON(consts.StatusBadGateway, map[string]string{"error": err.Error()})
		return
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
