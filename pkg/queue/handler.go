package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"time"

	"go.uber.org/zap"
)

// Default job types
const (
	JobTypeLLMCaching    = "llm_caching"
	JobTypeTokenCalc     = "token_calculation"
	JobTypeTenantSync    = "tenant_sync"
	JobTypeUsageTracking = "usage_tracking"
)

// JobHandlerFunc is a simple handler function type
type JobHandlerFunc func(ctx context.Context, job *Job) error

// Handle implements JobHandler interface
func (fn JobHandlerFunc) Handle(job *Job) error {
	return fn(context.Background(), job)
}

// TokenCalculationHandler calculates token usage for a request
func TokenCalculationHandler(queue *TaskQueue) JobHandler {
	return JobHandlerFunc(func(ctx context.Context, job *Job) error {
		zap.L().Info("processing token calculation", zap.String("job_id", job.ID))

		// Payload is map[string]interface{}, extract and convert to JSON
		payloadBytes, err := json.Marshal(job.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		var payload struct {
			Model    string `json:"model"`
			Prompt   string `json:"prompt"`
			Response string `json:"response"`
		}

		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		// Simulate token calculation (in real implementation, call tokenizer API)
		promptTokens := countTokens(payload.Prompt)
		responseTokens := countTokens(payload.Response)
		totalTokens := int64(promptTokens + responseTokens)

		zap.L().Info("token calculation complete",
			zap.String("model", payload.Model),
			zap.Int("prompt_tokens", promptTokens),
			zap.Int("response_tokens", responseTokens),
			zap.Int64("total_tokens", totalTokens),
		)

		// Track usage asynchronously
		if err := trackUsageAsync(queue, payload.Model, totalTokens); err != nil {
			zap.L().Warn("failed to track usage", zap.Error(err))
		}

		return nil
	})
}

// UsageTrackingHandler tracks tenant usage
func UsageTrackingHandler(queue *TaskQueue) JobHandler {
	return JobHandlerFunc(func(ctx context.Context, job *Job) error {
		zap.L().Info("processing usage tracking", zap.String("job_id", job.ID))

		payloadBytes, err := json.Marshal(job.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		var payload struct {
			TenantID uint   `json:"tenant_id"`
			Model    string `json:"model"`
			Tokens   int64  `json:"tokens"`
		}

		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		// In real implementation, update database with usage stats
		zap.L().Info("usage tracked",
			zap.Uint("tenant_id", payload.TenantID),
			zap.String("model", payload.Model),
			zap.Int64("tokens", payload.Tokens),
		)

		return nil
	})
}

// LLMCachingHandler handles caching LLM responses
func LLMCachingHandler() JobHandler {
	return JobHandlerFunc(func(ctx context.Context, job *Job) error {
		zap.L().Info("processing LLM caching", zap.String("job_id", job.ID))

		payloadBytes, err := json.Marshal(job.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		var payload struct {
			Prompt     string            `json:"prompt"`
			Model      string            `json:"model"`
			Response   string            `json:"response"`
			Metadata   map[string]string `json:"metadata"`
		}

		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		// Generate cache key based on prompt and model
		cacheKey := generateCacheKey(payload.Prompt, payload.Model)

		zap.L().Info("LLM response cached",
			zap.String("cache_key", cacheKey),
			zap.String("model", payload.Model),
		)

		// In real implementation, store in Redis with TTL
		// redisClient.Set(ctx, cacheKey, payload.Response, 24*time.Hour)

		return nil
	})
}

// TenantSyncHandler syncs tenant data
func TenantSyncHandler() JobHandler {
	return JobHandlerFunc(func(ctx context.Context, job *Job) error {
		zap.L().Info("processing tenant sync", zap.String("job_id", job.ID))

		payloadBytes, err := json.Marshal(job.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		var payload struct {
			TenantID uint   `json:"tenant_id"`
			Action   string `json:"action"` // create, update, delete
		}

		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return fmt.Errorf("failed to unmarshal payload: %w", err)
		}

		zap.L().Info("tenant synced",
			zap.Uint("tenant_id", payload.TenantID),
			zap.String("action", payload.Action),
		)

		return nil
	})
}

// Helper functions

func countTokens(text string) int {
	// Simple word-based approximation
	// In production, use proper tokenizer from OpenAI or other provider
	return len(text) / 4 // Rough estimate: 4 chars per token
}

func generateCacheKey(prompt, model string) string {
	// Generate cache key using content hash
	key := fmt.Sprintf("%s:%s", model, prompt)
	h := fnv.New32a()
	h.Write([]byte(key))
	return fmt.Sprintf("%x", h.Sum32())
}

func trackUsageAsync(queue *TaskQueue, model string, tokens int64) error {
	job := &Job{
		Type:       JobTypeUsageTracking,
		Payload:    map[string]interface{}{"model": model, "tokens": tokens},
		MaxRetries: 3,
		Timeout:    30 * time.Second,
	}

	return queue.Enqueue(job)
}
