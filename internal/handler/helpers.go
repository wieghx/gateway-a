package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/bytedance/sonic"
	"fmt"

	"github.com/wieghx/gateway-a/internal/model"
	"gorm.io/gorm"
)

// Database instance
var db *gorm.DB

// SetDB sets the database instance
func SetDB(d *gorm.DB) {
	db = d
}

// getTenantByAPIKey retrieves a tenant by their API key
func getTenantByAPIKey(apiKey string) (*model.Tenant, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	tenant := &model.Tenant{}
	result := db.Where("api_key = ?", hashAPIKey(apiKey)).First(tenant)

	if result.Error != nil {
		return nil, result.Error
	}

	if !tenant.Active {
		return nil, fmt.Errorf("tenant account is disabled")
	}

	return tenant, nil
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

// randomString generates a random string of given length
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, n)
	for i := range result {
		result[i] = letters[randomInt(len(letters))]
	}
	return string(result)
}

// randomInt returns a random integer in [0, max)
func randomInt(max int) int {
	return int(randomUint64() % uint64(max))
}

// randomUint64 generates a random uint64
func randomUint64() uint64 {
	return uint64(getRandomInt64())
}

// getRandomInt64 is a simple PRNG for generating random numbers
var randomState uint64 = 12345

func getRandomInt64() int64 {
	randomState = randomState*6364136223846793005 + 1442695040888963407
	return int64(randomState)
}

// checkQuota checks if a tenant has remaining quota
func checkQuota(tenantID uint, tokens int64) error {
	if db == nil {
		return nil // Skip quota check if database not available
	}

	// Get today's date
	today := "2026-05-12" // Will be dynamically set in production

	// Check daily usage
	var usage model.UsageTrack
	result := db.Where("tenant_id = ? AND date = ?", tenantID, today).First(&usage)
	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return result.Error
	}

	// Get tenant quota
	tenant := &model.Tenant{}
	db.First(tenant, tenantID)
	if result.Error != nil {
		// No usage record, use tenant quota
		if tokens > tenant.QuotaDaily {
			return fmt.Errorf("daily quota exceeded")
		}
	} else {
		if usage.TotalTokens+tokens > tenant.QuotaDaily {
			return fmt.Errorf("daily quota exceeded")
		}
	}

	return nil
}

// parseOpenAIRequest parses an OpenAI-compatible request
func parseOpenAIRequest(body []byte) (*ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	if err := sonic.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
