package audit

import (
	"context"
	"github.com/bytedance/sonic"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/zap"

	"github.com/wieghx/gateway-a/internal/logging"
)

// AuditLogger handles audit logging for sensitive operations
type AuditLogger struct {
	store           AuditStore
	sensitiveFields []string
	maxBodySize     int64
	enabled         bool
}

// AuditEvent represents a single audit log entry
type AuditEvent struct {
	Timestamp   string                 `json:"timestamp"`
	RequestID   string                 `json:"request_id"`
	UserID      string                 `json:"user_id"`
	IPAddress   string                 `json:"ip_address"`
	Action      string                 `json:"action"`
	Endpoint    string                 `json:"endpoint"`
	Method      string                 `json:"method"`
	StatusCode  int                    `json:"status_code"`
	Success     bool                   `json:"success"`
	Duration    int64                  `json:"duration_ms"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// AuditStore defines the interface for audit event storage
type AuditStore interface {
	Save(event *AuditEvent) error
	BatchSave(events []*AuditEvent) error
	GetEvents(userID string, startTime, endTime time.Time) ([]*AuditEvent, error)
	GetEventsByAction(action string, limit int) ([]*AuditEvent, error)
}

// FileAuditStore implements AuditStore using a file
type FileAuditStore struct {
	filePath string
	buffer   chan *AuditEvent
	done     chan struct{}
}

// InMemoryAuditStore implements AuditStore using in-memory storage
type InMemoryAuditStore struct {
	events    []*AuditEvent
	maxEvents int
}

// DBAuditStore implements AuditStore using a database (placeholder for future implementation)
type DBAuditStore struct {
	enabled bool
}

// Options contains audit logger configuration
type Options struct {
	StoreType       string
	FilePath        string
	BufferSize      int
	MaxBodySize     int64
	SensitiveFields []string
	Enabled         bool
}

// DefaultOptions returns default audit logger options
func DefaultOptions() Options {
	return Options{
		StoreType:       "memory",
		FilePath:        "/tmp/audit-logs.json",
		BufferSize:      1000,
		MaxBodySize:     10240, // 10KB
		SensitiveFields: []string{"password", "api_key", "secret", "token", "authorization"},
		Enabled:         true,
	}
}

// NewAuditLogger creates a new audit logger with the given options
func NewAuditLogger(opts Options) *AuditLogger {
	store := createStore(opts)

	return &AuditLogger{
		store:           store,
		sensitiveFields: opts.SensitiveFields,
		maxBodySize:     opts.MaxBodySize,
		enabled:         opts.Enabled,
	}
}

// createStore creates the appropriate audit store based on options
func createStore(opts Options) AuditStore {
	switch opts.StoreType {
	case "file":
		return NewFileAuditStore(opts.FilePath, opts.BufferSize)
	case "database":
		return &DBAuditStore{enabled: true}
	case "memory", "":
		fallthrough
	default:
		return NewInMemoryAuditStore(10000)
	}
}

// NewFileAuditStore creates a file-based audit store
func NewFileAuditStore(filePath string, bufferSize int) *FileAuditStore {
	store := &FileAuditStore{
		filePath: filePath,
		buffer:   make(chan *AuditEvent, bufferSize),
		done:     make(chan struct{}),
	}
	// Start background writer
	go store.writeLoop()
	return store
}

// writeLoop writes events to file asynchronously
func (f *FileAuditStore) writeLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var buffer []*AuditEvent

	write := func() {
		if len(buffer) == 0 {
			return
		}

		data, err := sonic.MarshalIndent(buffer, "", "  ")
		if err != nil {
			logging.Logger.Error("Failed to marshal audit events", zap.Error(err))
			return
		}

		// Append to file
		file, err := os.OpenFile(f.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logging.Logger.Error("Failed to open audit log file", zap.Error(err))
			return
		}
		defer file.Close()

		_, err = file.Write(data)
		if err != nil {
			logging.Logger.Error("Failed to write audit log", zap.Error(err))
		}

		buffer = buffer[:0]
	}

	for {
		select {
		case <-ticker.C:
			write()
		case event, ok := <-f.buffer:
			if !ok {
				return
			}
			buffer = append(buffer, event)
			if len(buffer) >= cap(f.buffer) {
				write()
			}
		case <-f.done:
			write()
			close(f.buffer)
			return
		}
	}
}

// Save saves a single audit event
func (f *FileAuditStore) Save(event *AuditEvent) error {
	select {
	case f.buffer <- event:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("audit log buffer full")
	}
}

// BatchSave saves multiple audit events
func (f *FileAuditStore) BatchSave(events []*AuditEvent) error {
	for _, event := range events {
		if err := f.Save(event); err != nil {
			return err
		}
	}
	return nil
}

// GetEvents retrieves audit events by user ID and time range
func (f *FileAuditStore) GetEvents(userID string, startTime, endTime time.Time) ([]*AuditEvent, error) {
	// File-based implementation would read from file
	// For now, return empty
	return []*AuditEvent{}, nil
}

// GetEventsByAction retrieves audit events by action type
func (f *FileAuditStore) GetEventsByAction(action string, limit int) ([]*AuditEvent, error) {
	return []*AuditEvent{}, nil
}

// Close closes the file audit store
func (f *FileAuditStore) Close() error {
	close(f.done)
	return nil
}

// NewInMemoryAuditStore creates an in-memory audit store
func NewInMemoryAuditStore(maxEvents int) *InMemoryAuditStore {
	return &InMemoryAuditStore{
		events:    make([]*AuditEvent, 0),
		maxEvents: maxEvents,
	}
}

// Save saves an audit event to memory
func (m *InMemoryAuditStore) Save(event *AuditEvent) error {
	m.events = append(m.events, event)

	// Trim if over limit
	if len(m.events) > m.maxEvents {
		m.events = m.events[len(m.events)-m.maxEvents:]
	}

	return nil
}

// BatchSave saves multiple events
func (m *InMemoryAuditStore) BatchSave(events []*AuditEvent) error {
	for _, event := range events {
		if err := m.Save(event); err != nil {
			return err
		}
	}
	return nil
}

// GetEvents retrieves events by user ID and time range
func (m *InMemoryAuditStore) GetEvents(userID string, startTime, endTime time.Time) ([]*AuditEvent, error) {
	var results []*AuditEvent
	for _, event := range m.events {
		if event.UserID == userID {
			eventTime, _ := time.Parse(time.RFC3339, event.Timestamp)
			if !eventTime.Before(startTime) && !eventTime.After(endTime) {
				results = append(results, event)
			}
		}
	}
	return results, nil
}

// GetEventsByAction retrieves events by action type
func (m *InMemoryAuditStore) GetEventsByAction(action string, limit int) ([]*AuditEvent, error) {
	var results []*AuditEvent
	for _, event := range m.events {
		if event.Action == action {
			results = append(results, event)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// GetAllEvents retrieves all events
func (m *InMemoryAuditStore) GetAllEvents() []*AuditEvent {
	return m.events
}

// Clear clears all events
func (m *InMemoryAuditStore) Clear() {
	m.events = m.events[:0]
}

// NewDBAuditStore creates a database-backed audit store (placeholder)
func NewDBAuditStore() *DBAuditStore {
	return &DBAuditStore{enabled: true}
}

// Save saves an audit event to database (placeholder)
func (d *DBAuditStore) Save(event *AuditEvent) error {
	if !d.enabled {
		return nil
	}
	// Future implementation
	return nil
}

// BatchSave saves multiple events to database (placeholder)
func (d *DBAuditStore) BatchSave(events []*AuditEvent) error {
	if !d.enabled {
		return nil
	}
	return nil
}

// GetEvents retrieves events from database (placeholder)
func (d *DBAuditStore) GetEvents(userID string, startTime, endTime time.Time) ([]*AuditEvent, error) {
	return []*AuditEvent{}, nil
}

// GetEventsByAction retrieves events by action (placeholder)
func (d *DBAuditStore) GetEventsByAction(action string, limit int) ([]*AuditEvent, error) {
	return []*AuditEvent{}, nil
}

// ShouldAudit determines if an action should be audited
func (a *AuditLogger) ShouldAudit(action string) bool {
	// Admin actions should always be audited
	adminActions := []string{
		"create_tenant", "update_tenant", "delete_tenant",
		"create_api_key", "revoke_api_key", "update_api_key",
		"create_user", "update_user", "delete_user",
		"reset_password", "grant_permission", "revoke_permission",
		"update_config", "create_plugin", "delete_plugin",
		"enable_feature", "disable_feature",
		"approve_request", "reject_request", "suspend_user",
	}

	for _, adminAction := range adminActions {
		if action == adminAction {
			return a.enabled
		}
	}

	return false
}

// CreateEvent creates a new audit event
func (a *AuditLogger) CreateEvent(c context.Context, ctx *app.RequestContext, action string, success bool, metadata map[string]interface{}) *AuditEvent {
	event := &AuditEvent{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		RequestID:   logging.GetRequestID(c),
		Action:      action,
		StatusCode:  0, // Will be set after response
		Success:     success,
		Duration:    0, // Will be calculated
		Metadata:    metadata,
	}

	// Set IP and endpoint only if context is not nil
	if ctx != nil {
		event.IPAddress = ctx.ClientIP()
		event.Endpoint = strings.TrimSpace(string(ctx.Request.URI().Path())) + "?" + string(ctx.Request.URI().QueryString())
		event.Method = string(ctx.Request.Method())

		// Try to get user ID from context
		if userID, ok := ctx.Get("user_id"); ok {
			if str, ok := userID.(string); ok {
				event.UserID = str
			}
		} else if apiKey, ok := ctx.Get("api_key"); ok {
			if str, ok := apiKey.(string); ok {
				event.UserID = str
			}
		}
	}

		return event
}

// RedactFields redacts sensitive fields from a map (exported for testing)
func (a *AuditLogger) RedactFields(data map[string]interface{}) map[string]interface{} {
	return a.redactFields(data)
}

// redactFields redacts sensitive fields from a map
func (a *AuditLogger) redactFields(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range data {
		lowerKey := strings.ToLower(key)
		isSensitive := false

		for _, field := range a.sensitiveFields {
			if strings.Contains(lowerKey, field) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			result[key] = "***REDACTED***"
		} else {
			result[key] = value
		}
	}

	return result
}

// Middleware creates an audit logging middleware
func Middleware(logger *AuditLogger, actions []string) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		// Track request start time
		startTime := time.Now()

		// Capture request body for auditing
		var requestBody []byte
		rawBody := ctx.Request.BodyBytes()
		if len(rawBody) > 0 && int64(len(rawBody)) <= logger.maxBodySize {
			requestBody = make([]byte, len(rawBody))
			copy(requestBody, rawBody)
		}

		// Capture response status
		var statusCode int

		// Execute handler
		ctx.Next(c)

		// Calculate duration
		duration := time.Since(startTime).Milliseconds()

		// Get response status
		statusCode = ctx.Response.StatusCode()

		// Get action from context (set by route handler)
		action, ok := ctx.Get("audit_action")
		if !ok {
			return
		}

		actionStr, ok := action.(string)
		if !ok {
			return
		}

		// Check if this action should be audited
		if !logger.ShouldAudit(actionStr) {
			return
		}

		// Create audit event
		var input map[string]interface{}
		if len(requestBody) > 0 {
			sonic.Unmarshal(requestBody, &input)
		}

		// Get user info from context
		userID := "anonymous"
		if u, ok := ctx.Get("user_id"); ok {
			if str, ok := u.(string); ok {
				userID = str
			}
		} else if k, ok := ctx.Get("api_key"); ok {
			if str, ok := k.(string); ok {
				userID = str
			}
		}

		event := &AuditEvent{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			RequestID:   logging.GetRequestID(c),
			UserID:      userID,
			IPAddress:   ctx.ClientIP(),
			Action:      actionStr,
			Endpoint:    strings.TrimSpace(string(ctx.Request.URI().Path())) + "?" + string(ctx.Request.URI().QueryString()),
			Method:      string(ctx.Request.Method()),
			StatusCode:  statusCode,
			Success:     statusCode < 400,
			Duration:    duration,
			Metadata:    map[string]interface{}{},
			Input:       logger.RedactFields(input),
		}

		// Log the event
		if err := logger.store.Save(event); err != nil {
			logging.Logger.Error("Failed to log audit event",
				logging.WithRequestID(logging.GetRequestID(c)),
				zap.String("action", actionStr),
				zap.Error(err),
			)
		}
	}
}

// AuditAction sets the audit action in the context
func AuditAction(action string) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		ctx.Set("audit_action", action)
		ctx.Next(c)
	}
}

// ExtractUserFromContext extracts user information from the request context
func ExtractUserFromContext(ctx *app.RequestContext) string {
	// Try different sources for user ID
	if userID, ok := ctx.Get("user_id"); ok {
		if str, ok := userID.(string); ok {
			return str
		}
	}

	if apiKey, ok := ctx.Get("api_key"); ok {
		if str, ok := apiKey.(string); ok {
			return str
		}
	}

	// Check authorization header
	authHeader := ctx.Request.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}

	return "anonymous"
}

// ExtractRequestDetails extracts request details for auditing
func ExtractRequestDetails(ctx *app.RequestContext) (string, map[string]interface{}) {
	var input map[string]interface{}

	// Try to parse request body
	body := ctx.Request.BodyBytes()
	if len(body) > 0 {
		sonic.Unmarshal(body, &input)
	}

	// Redact sensitive fields
	if input != nil {
		for key := range input {
			lowerKey := strings.ToLower(key)
			if strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "token") {
				input[key] = "***REDACTED***"
			}
		}
	}

	return string(ctx.Request.Method()), input
}

// GetRequestURL constructs the full request URL
func GetRequestURL(ctx *app.RequestContext) string {
	scheme := "http"
	// Hertz does not expose TLS directly, assume HTTP for now
	// TLS can be detected via X-Forwarded-Proto header
	if proto := ctx.Request.Header.Get("X-Forwarded-Proto"); proto == "https" {
		scheme = "https"
	}

	host := string(ctx.Request.Host())
	path := string(ctx.Request.URI().Path())
	query := string(ctx.Request.URI().QueryString())

	urlStr := fmt.Sprintf("%s://%s%s", scheme, host, path)
	if query != "" {
		urlStr += "?" + query
	}

	return urlStr
}

// SanitizeInput sanitizes request input for safe logging
func SanitizeInput(input map[string]interface{}, sensitiveFields []string) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range input {
		lowerKey := strings.ToLower(key)
		isSensitive := false

		for _, field := range sensitiveFields {
			if strings.Contains(lowerKey, field) {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			result[key] = "***REDACTED***"
		} else {
			result[key] = value
		}
	}

	return result
}
