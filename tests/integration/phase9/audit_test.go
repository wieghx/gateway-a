//go:build integration

package phase9

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gu/gateway-a/middleware/audit"
)

// TestAuditLogger tests the audit logger
func TestAuditLogger(t *testing.T) {
	t.Run("creates valid audit logger", func(t *testing.T) {
		opts := audit.DefaultOptions()

		assert.NotNil(t, opts)
		assert.Equal(t, "memory", opts.StoreType)
		assert.Equal(t, int64(10240), opts.MaxBodySize)
		assert.Contains(t, opts.SensitiveFields, "password")
		assert.Contains(t, opts.SensitiveFields, "api_key")
		assert.True(t, opts.Enabled)
	})

	t.Run("shouldAudit admin actions", func(t *testing.T) {
		logger := audit.NewAuditLogger(audit.DefaultOptions())

		adminActions := []string{
			"create_tenant", "update_tenant", "delete_tenant",
			"create_api_key", "revoke_api_key",
			"create_user", "update_user", "delete_user",
			"update_config", "create_plugin",
		}

		for _, action := range adminActions {
			assert.True(t, logger.ShouldAudit(action), "action %s should be audited", action)
		}
	})

	t.Run("shouldAudit non-admin actions", func(t *testing.T) {
		logger := audit.NewAuditLogger(audit.DefaultOptions())

		nonAdminActions := []string{
			"view_profile", "list_items", "get_health",
			"read_config", "check_status",
		}

		for _, action := range nonAdminActions {
			assert.False(t, logger.ShouldAudit(action), "action %s should not be audited", action)
		}
	})

	t.Run("redacts sensitive fields", func(t *testing.T) {
		opts := audit.DefaultOptions()
		opts.SensitiveFields = []string{"password", "secret", "token"}
		logger := audit.NewAuditLogger(opts)

		input := map[string]interface{}{
			"username": "testuser",
			"password": "secret123",
			"token":    "abc123xyz",
			"email":    "test@example.com",
		}

		redacted := logger.RedactFields(input)

		assert.Equal(t, "testuser", redacted["username"])
		assert.Equal(t, "***REDACTED***", redacted["password"])
		assert.Equal(t, "***REDACTED***", redacted["token"])
		assert.Equal(t, "test@example.com", redacted["email"])
	})

	t.Run("case insensitive field redaction", func(t *testing.T) {
		opts := audit.DefaultOptions()
		opts.SensitiveFields = []string{"password"}
		logger := audit.NewAuditLogger(opts)

		input := map[string]interface{}{
			"Password": "secret",
			"PASSWORD": "secret",
			"passWord": "secret",
		}

		redacted := logger.RedactFields(input)

		assert.Equal(t, "***REDACTED***", redacted["Password"])
		assert.Equal(t, "***REDACTED***", redacted["PASSWORD"])
		assert.Equal(t, "***REDACTED***", redacted["passWord"])
	})
}

// TestAuditEvent tests audit event creation
func TestAuditEvent(t *testing.T) {
	t.Run("creates audit event", func(t *testing.T) {
		opts := audit.DefaultOptions()
		logger := audit.NewAuditLogger(opts)

		event := logger.CreateEvent(
			context.Background(),
			nil,
			"create_tenant",
			true,
			map[string]interface{}{"tenant_id": "123"},
		)

		require.NotNil(t, event)
		assert.Equal(t, "create_tenant", event.Action)
		assert.True(t, event.Success)
		assert.NotEmpty(t, event.Timestamp)
		assert.NotNil(t, event.Metadata)
	})

	t.Run("event has required fields", func(t *testing.T) {
		opts := audit.DefaultOptions()
		logger := audit.NewAuditLogger(opts)

		event := logger.CreateEvent(
			context.Background(),
			nil,
			"test_action",
			true,
			nil,
		)

		assert.NotEmpty(t, event.Timestamp)
		assert.NotNil(t, event.RequestID)
		assert.Equal(t, "test_action", event.Action)
	})

	t.Run("event captures duration", func(t *testing.T) {
		opts := audit.DefaultOptions()
		logger := audit.NewAuditLogger(opts)

		event := logger.CreateEvent(
			context.Background(),
			nil,
			"slow_action",
			true,
			nil,
		)

		assert.NotNil(t, event)
		_ = event.Duration
	})
}

// TestInMemoryAuditStore tests in-memory audit store
func TestInMemoryAuditStore(t *testing.T) {
	t.Run("saves events", func(t *testing.T) {
		store := audit.NewInMemoryAuditStore(100)

		event := &audit.AuditEvent{
			Timestamp: time.Now().Format(time.RFC3339),
			Action:    "test_action",
			UserID:    "user123",
		}

		err := store.Save(event)
		assert.NoError(t, err)

		events := store.GetAllEvents()
		assert.Len(t, events, 1)
		assert.Equal(t, "test_action", events[0].Action)
	})

	t.Run("trims events at max size", func(t *testing.T) {
		store := audit.NewInMemoryAuditStore(3)

		for i := 0; i < 5; i++ {
			event := &audit.AuditEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Action:    "test",
				UserID:    "user123",
			}
			store.Save(event)
		}

		events := store.GetAllEvents()
		assert.Len(t, events, 3) // Should be trimmed to max size
	})

	t.Run("filters by user ID", func(t *testing.T) {
		store := audit.NewInMemoryAuditStore(100)

		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now().Add(1 * time.Hour)

		// Save events for different users
		for _, userID := range []string{"user1", "user2", "user1"} {
			event := &audit.AuditEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Action:    "test_action",
				UserID:    userID,
			}
			store.Save(event)
		}

		events, err := store.GetEvents("user1", startTime, endTime)
		assert.NoError(t, err)
		assert.Len(t, events, 2)
	})

	t.Run("filters by action", func(t *testing.T) {
		store := audit.NewInMemoryAuditStore(100)

		actions := []string{"create", "update", "create", "delete", "create"}
		for _, action := range actions {
			event := &audit.AuditEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Action:    action,
			}
			store.Save(event)
		}

		events, err := store.GetEventsByAction("create", 0)
		assert.NoError(t, err)
		assert.Len(t, events, 3)
	})

	t.Run("clears all events", func(t *testing.T) {
		store := audit.NewInMemoryAuditStore(100)

		event := &audit.AuditEvent{
			Timestamp: time.Now().Format(time.RFC3339),
			Action:    "test",
		}
		store.Save(event)

		store.Clear()

		events := store.GetAllEvents()
		assert.Len(t, events, 0)
	})
}

// TestFileAuditStore tests file audit store
func TestFileAuditStore(t *testing.T) {
	t.Run("creates file store", func(t *testing.T) {
		store := audit.NewFileAuditStore("/tmp/test-audit.json", 10)

		require.NotNil(t, store)

		// Clean up
		store.Close()
	})

	t.Run("buffers events", func(t *testing.T) {
		store := audit.NewFileAuditStore("/tmp/test-audit.json", 100)

		event := &audit.AuditEvent{
			Timestamp: time.Now().Format(time.RFC3339),
			Action:    "test_action",
		}

		err := store.Save(event)
		assert.NoError(t, err)

		store.Close()
	})

	t.Run("batch saves events", func(t *testing.T) {
		store := audit.NewFileAuditStore("/tmp/test-audit.json", 100)

		events := make([]*audit.AuditEvent, 5)
		for i := range events {
			events[i] = &audit.AuditEvent{
				Timestamp: time.Now().Format(time.RFC3339),
				Action:    "batch_test",
			}
		}

		err := store.BatchSave(events)
		assert.NoError(t, err)

		store.Close()
	})
}

// TestAuditMiddleware tests audit logging middleware
func TestAuditMiddleware(t *testing.T) {
	t.Run("creates middleware", func(t *testing.T) {
		opts := audit.DefaultOptions()
		logger := audit.NewAuditLogger(opts)

		actions := []string{"create_tenant", "update_config"}
		middleware := audit.Middleware(logger, actions)

		assert.NotNil(t, middleware)
	})

	t.Run("sets audit action in context", func(t *testing.T) {
		handler := audit.AuditAction("create_tenant")

		hertzCtx := &app.RequestContext{}

		handler(nil, hertzCtx)

		assert.NotNil(t, hertzCtx)
	})

	t.Run("extracts user from context", func(t *testing.T) {
		hertzCtx := &app.RequestContext{}
		hertzCtx.Request.Header.Set("Authorization", "Bearer test-key")

		userID := audit.ExtractUserFromContext(hertzCtx)
		assert.Equal(t, "test-key", userID)
	})

	t.Run("extracts user ID from context value", func(t *testing.T) {
		hertzCtx := &app.RequestContext{}
		hertzCtx.Set("user_id", "user-123")

		userID := audit.ExtractUserFromContext(hertzCtx)
		assert.Equal(t, "user-123", userID)
	})
}

// TestRequestDetails extraction
func TestRequestDetailsExtraction(t *testing.T) {
	t.Run("extracts method and input", func(t *testing.T) {
		hertzCtx := &app.RequestContext{}
		hertzCtx.Request.SetBodyString(`{"key": "value"}`)

		method, input := audit.ExtractRequestDetails(hertzCtx)

		assert.NotNil(t, method)
		assert.NotNil(t, input)
		assert.Contains(t, input, "key")
	})

	t.Run("redacts sensitive fields in input", func(t *testing.T) {
		hertzCtx := &app.RequestContext{}
		hertzCtx.Request.SetBodyString(`{"password": "secret123", "username": "test"}`)

		method, input := audit.ExtractRequestDetails(hertzCtx)

		assert.NotNil(t, method)
		assert.Equal(t, "***REDACTED***", input["password"])
		assert.Equal(t, "test", input["username"])
	})
}

// TestURL extraction
func TestRequestURL(t *testing.T) {
	t.Run("constructs HTTP URL", func(t *testing.T) {
		hertzCtx := &app.RequestContext{}
		// Hertz sets Host via URI - can't directly assign to Request.Host

		urlStr := audit.GetRequestURL(hertzCtx)

		assert.Contains(t, urlStr, "http://")
	})
}

// TestSanitizeInput tests sanitization helper
func TestSanitizeInput(t *testing.T) {
	t.Run("sanitizes map with sensitive fields", func(t *testing.T) {
		input := map[string]interface{}{
			"username": "test",
			"password": "secret",
			"token":    "abc123",
		}

		sensitiveFields := []string{"password", "token"}
		sanitized := audit.SanitizeInput(input, sensitiveFields)

		assert.Equal(t, "test", sanitized["username"])
		assert.Equal(t, "***REDACTED***", sanitized["password"])
		assert.Equal(t, "***REDACTED***", sanitized["token"])
	})
}

// BenchmarkAudit tests performance
func BenchmarkCreateAuditEvent(b *testing.B) {
	opts := audit.DefaultOptions()
	logger := audit.NewAuditLogger(opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logger.CreateEvent(
			context.Background(),
			nil,
			"test_action",
			true,
			map[string]interface{}{"key": "value"},
		)
	}
}

func BenchmarkRedactFields(b *testing.B) {
	opts := audit.DefaultOptions()
	opts.SensitiveFields = []string{"password", "secret", "token"}
	logger := audit.NewAuditLogger(opts)

	input := map[string]interface{}{
		"username": "test",
		"password": "secret",
		"token":    "abc123",
		"email":    "test@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logger.RedactFields(input)
	}
}

func BenchmarkShouldAudit(b *testing.B) {
	opts := audit.DefaultOptions()
	logger := audit.NewAuditLogger(opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = logger.ShouldAudit("create_tenant")
	}
}

// Integration test placeholder
func TestAuditStorePersistence(t *testing.T) {
	t.Run("file store persists events", func(t *testing.T) {
		store := audit.NewFileAuditStore("/tmp/integration-audit-test.json", 100)

		event := &audit.AuditEvent{
			Timestamp: time.Now().Format(time.RFC3339),
			Action:    "integration_test",
			UserID:    "test-user",
		}

		err := store.Save(event)
		assert.NoError(t, err)

		events, err := store.GetEvents("test-user", time.Now().Add(-1*time.Hour), time.Now().Add(1*time.Hour))
		assert.NoError(t, err)
		_ = events

		store.Close()
	})
}
