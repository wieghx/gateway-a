package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/wieghx/gateway-a/internal/model"
	"github.com/wieghx/gateway-a/internal/tokenizer"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Database instance
var db *gorm.DB

// Prometheus metrics for quota/usage observability (defined here for direct access from quota logic)
var (
	promTenantUsage = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_tenant_usage_tokens_total",
		Help: "Total tokens used by tenant per model",
	}, []string{"tenant_id", "model", "type"}) // prompt, completion, total

	promQuotaExceeded = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_quota_exceeded_total",
		Help: "Total quota exceeded events",
	}, []string{"tenant_id", "quota_type"}) // daily, monthly

	promUsageRecordFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_usage_record_failures_total",
		Help: "Total failures when recording usage to DB",
	}, []string{"operation"}) // create, update
)

func init() {
	prometheus.MustRegister(promTenantUsage)
	prometheus.MustRegister(promQuotaExceeded)
	prometheus.MustRegister(promUsageRecordFailures)
}

// SetDB sets the database instance
func SetDB(d *gorm.DB) {
	db = d
}

// getTenantByAPIKey retrieves a tenant by their API key.
// It now correctly uses the api_keys table (preferred) while maintaining
// basic compatibility during transition.
func getTenantByAPIKey(apiKey string) (*model.Tenant, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	hashed := hashAPIKey(apiKey)

	// Preferred path: lookup in api_keys table
	var apiKeyRecord model.APIKey
	if err := db.Where("key_hash = ? AND active = ?", hashed, true).First(&apiKeyRecord).Error; err == nil {
		tenant := &model.Tenant{}
		if err := db.First(tenant, apiKeyRecord.TenantID).Error; err != nil {
			return nil, err
		}
		if !tenant.Active {
			return nil, fmt.Errorf("tenant account is disabled")
		}
		return tenant, nil
	}

	// Legacy fallback (for old data where Tenant still had direct api_key)
	// This can be removed once migration is complete.
	tenant := &model.Tenant{}
	if err := db.Where("api_key = ?", hashed).First(tenant).Error; err == nil {
		if !tenant.Active {
			return nil, fmt.Errorf("tenant account is disabled")
		}
		return tenant, nil
	}

	return nil, fmt.Errorf("invalid api key")
}

// hashAPIKey hashes an API key for storage and comparison
func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// createAPIKey creates a new API key for a tenant
func createAPIKey(tenantID uint, keyName string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	// Generate new key
	newKey := generateAPIKey()
	hashedKey := hashAPIKey(newKey)

	apiKey := &model.APIKey{
		TenantID: tenantID,
		KeyHash:  hashedKey,
		Name:     keyName,
		Active:   true,
	}

	result := db.Create(apiKey)
	if result.Error != nil {
		return "", result.Error
	}

	return newKey, nil
}

// generateAPIKey generates a new API key
func generateAPIKey() string {
	return "sk-" + randomString(32)
}

// randomString generates a cryptographically secure random string of given length
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, n)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			// Fallback should never be needed, but avoid panic in hot path
			result[i] = letters[i%len(letters)]
			continue
		}
		result[i] = letters[num.Int64()]
	}
	return string(result)
}

// checkQuota checks if a tenant has remaining quota.
// This is a best-effort pre-flight check. Real cumulative enforcement
// improves after recordUsage writes actual token counts.
func checkQuota(tenantID uint, tokens int64) error {
	if db == nil {
		return nil // Skip quota check if database not available
	}

	today := time.Now().Format("2006-01-02")

	// Always fetch latest usage for reliability
	var usage model.UsageTrack
	result := db.Where("tenant_id = ? AND date = ?", tenantID, today).First(&usage)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return result.Error
	}

	tenant := &model.Tenant{}
	if err := db.First(tenant, tenantID).Error; err != nil {
		return fmt.Errorf("failed to load tenant: %w", err)
	}

	tenantIDStr := fmt.Sprintf("%d", tenantID)
	hasUsage := result.Error == nil

	var currentTotal int64
	if hasUsage {
		currentTotal = usage.TotalTokens
	}

	wouldExceed := currentTotal+tokens > tenant.QuotaDaily

	if wouldExceed {
		promQuotaExceeded.WithLabelValues(tenantIDStr, "daily").Inc()
		zap.L().Warn("quota exceeded (pre-check)",
			zap.Uint("tenant_id", tenantID),
			zap.Int64("current_usage", currentTotal),
			zap.Int64("requested", tokens),
			zap.Int64("quota", tenant.QuotaDaily),
		)
		return fmt.Errorf("daily quota exceeded")
	}

	return nil
}

// extractUsageFromStreamBuffer attempts to find and parse usage information
// from a buffer of raw SSE streaming chunks.
//
// OpenAI (and compatible providers) may emit a final chunk containing
// "usage" when the client requests stream_options.include_usage.
// This function does a best-effort scan for such usage objects.
func extractUsageFromStreamBuffer(data []byte) Usage {
	if len(data) == 0 {
		return Usage{}
	}

	// Split into lines and look for JSON objects containing "usage"
	lines := bytes.Split(data, []byte("\n"))

	// Scan from the end (last chunks are more likely to contain usage)
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		// Many SSE lines are "data: {json}"
		if idx := bytes.Index(line, []byte("data: ")); idx != -1 {
			line = line[idx+6:]
		}

		// Skip [DONE] and empty markers
		if bytes.Contains(line, []byte("[DONE]")) || len(line) < 10 {
			continue
		}

		var raw map[string]json.RawMessage
		if err := sonic.Unmarshal(line, &raw); err != nil {
			continue
		}

		if usageRaw, ok := raw["usage"]; ok && len(usageRaw) > 2 {
			var u Usage
			if err := sonic.Unmarshal(usageRaw, &u); err == nil && u.TotalTokens > 0 {
				return u
			}
		}
	}

	return Usage{}
}

// estimateTokensRough is kept only for backward compatibility with existing tests.
// New code should import "github.com/wieghx/gateway-a/internal/tokenizer" instead.
func estimateTokensRough(text string) int64 {
	return tokenizer.EstimateTokensRough(text)
}

// recordUsage records the actual token usage from an upstream response.
// This is the foundation for real quota enforcement and billing.
// 
// It is now non-blocking (runs in background) and has retry logic
// to be resilient without impacting user-facing latency.
func recordUsage(tenantID uint, modelName string, u Usage) {
	if db == nil || u.TotalTokens == 0 {
		return
	}

	// Fire and forget to avoid blocking the response
	go func() {
		recordUsageWithRetry(tenantID, modelName, u)
	}()
}

// recordUsageWithRetry performs the actual DB write with retries.
// On final failure, it writes to a simple local dead-letter file for later recovery.
func recordUsageWithRetry(tenantID uint, modelName string, u Usage) {
	const maxAttempts = 3

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
		}

		today := time.Now().Format("2006-01-02")

		result := db.Model(&model.UsageTrack{}).
			Where("tenant_id = ? AND date = ? AND model = ?", tenantID, today, modelName).
			Updates(map[string]interface{}{
				"prompt_tokens":     gorm.Expr("prompt_tokens + ?", u.PromptTokens),
				"completion_tokens": gorm.Expr("completion_tokens + ?", u.CompletionTokens),
				"total_tokens":      gorm.Expr("total_tokens + ?", u.TotalTokens),
			})

		if result.Error != nil {
			if attempt == maxAttempts-1 {
				log.Printf("recordUsage failed after retries: %v", result.Error)
				promUsageRecordFailures.WithLabelValues("update").Inc()
				writeUsageDeadLetter(tenantID, modelName, u, result.Error)
			}
			continue
		}

		if result.RowsAffected == 0 {
			track := &model.UsageTrack{
				TenantID:         tenantID,
				Date:             today,
				Model:            modelName,
				PromptTokens:     u.PromptTokens,
				CompletionTokens: u.CompletionTokens,
				TotalTokens:      u.TotalTokens,
			}
			if err := db.Create(track).Error; err != nil {
				if attempt == maxAttempts-1 {
					log.Printf("recordUsage create failed after retries: %v", err)
					promUsageRecordFailures.WithLabelValues("create").Inc()
					writeUsageDeadLetter(tenantID, modelName, u, err)
				}
				continue
			}
		}

		// Success
		tenantStr := fmt.Sprintf("%d", tenantID)
		promTenantUsage.WithLabelValues(tenantStr, modelName, "prompt").Add(float64(u.PromptTokens))
		promTenantUsage.WithLabelValues(tenantStr, modelName, "completion").Add(float64(u.CompletionTokens))
		promTenantUsage.WithLabelValues(tenantStr, modelName, "total").Add(float64(u.TotalTokens))

		zap.L().Info("usage recorded",
			zap.Uint("tenant_id", tenantID),
			zap.String("model", modelName),
			zap.Int64("prompt_tokens", u.PromptTokens),
			zap.Int64("completion_tokens", u.CompletionTokens),
			zap.Int64("total_tokens", u.TotalTokens),
		)
		return
	}
}

// writeUsageDeadLetter writes failed usage records to a local file for manual or later recovery.
// Records are written as JSON Lines for easier parsing.
func writeUsageDeadLetter(tenantID uint, modelName string, u Usage, err error) {
	record := struct {
		Timestamp        string `json:"timestamp"`
		TenantID         uint   `json:"tenant_id"`
		Model            string `json:"model"`
		PromptTokens     int64  `json:"prompt_tokens"`
		CompletionTokens int64  `json:"completion_tokens"`
		TotalTokens      int64  `json:"total_tokens"`
		Error            string `json:"error"`
	}{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		TenantID:         tenantID,
		Model:            modelName,
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		Error:            err.Error(),
	}

	data, _ := sonic.Marshal(record)
	line := string(data) + "\n"

	f, errOpen := os.OpenFile("usage_deadletter.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if errOpen == nil {
		_, _ = f.WriteString(line)
		f.Close()
	}
}

// ProcessUsageDeadLetter attempts to re-process records in the dead-letter file.
// It parses JSON Lines and tries to write them back to the database.
// Successfully processed lines are removed from the file (best-effort).
func ProcessUsageDeadLetter() (processed int, remaining int, err error) {
	if db == nil {
		return 0, 0, fmt.Errorf("database not available")
	}

	data, err := os.ReadFile("usage_deadletter.log")
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	lines := strings.Split(string(data), "\n")
	var keptLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var rec struct {
			TenantID         uint   `json:"tenant_id"`
			Model            string `json:"model"`
			PromptTokens     int64  `json:"prompt_tokens"`
			CompletionTokens int64  `json:"completion_tokens"`
			TotalTokens      int64  `json:"total_tokens"`
		}

		if err := sonic.Unmarshal([]byte(line), &rec); err != nil {
			// Keep unparseable lines
			keptLines = append(keptLines, line)
			remaining++
			continue
		}

		u := Usage{
			PromptTokens:     rec.PromptTokens,
			CompletionTokens: rec.CompletionTokens,
			TotalTokens:      rec.TotalTokens,
		}

		// Try to record again
		today := time.Now().Format("2006-01-02")
		result := db.Model(&model.UsageTrack{}).
			Where("tenant_id = ? AND date = ? AND model = ?", rec.TenantID, today, rec.Model).
			Updates(map[string]interface{}{
				"prompt_tokens":     gorm.Expr("prompt_tokens + ?", u.PromptTokens),
				"completion_tokens": gorm.Expr("completion_tokens + ?", u.CompletionTokens),
				"total_tokens":      gorm.Expr("total_tokens + ?", u.TotalTokens),
			})

		if result.Error != nil || result.RowsAffected == 0 {
			// Keep the line for next attempt
			keptLines = append(keptLines, line)
			remaining++
		} else {
			processed++
		}
	}

	// Rewrite the file with only the remaining lines
	newContent := strings.Join(keptLines, "\n")
	if len(keptLines) > 0 {
		newContent += "\n"
	}
	_ = os.WriteFile("usage_deadletter.log", []byte(newContent), 0644)

	return processed, remaining, nil
}

// parseOpenAIRequest parses an OpenAI-compatible request
func parseOpenAIRequest(body []byte) (*ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	if err := sonic.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// getQuotaStatus returns current daily usage and remaining quota for a tenant.
// Useful for response headers and observability.
func getQuotaStatus(tenantID uint) (used int64, remaining int64, limit int64) {
	if db == nil {
		return 0, 0, 0
	}

	today := time.Now().Format("2006-01-02")
	tenant := &model.Tenant{}
	if err := db.First(tenant, tenantID).Error; err != nil {
		return 0, 0, 0
	}

	var usage model.UsageTrack
	db.Where("tenant_id = ? AND date = ?", tenantID, today).First(&usage)

	used = usage.TotalTokens
	limit = tenant.QuotaDaily
	remaining = limit - used
	if remaining < 0 {
		remaining = 0
	}
	return used, remaining, limit
}
