package validation

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"

	"github.com/wieghx/gateway-a/internal/logging"
)

// Validator performs request validation
type Validator struct {
	// Pattern validators
	emailPattern    *regexp.Regexp
	urlPattern      *regexp.Regexp
	ipPattern       *regexp.Regexp
	ipv6Pattern     *regexp.Regexp
	hostnamePattern *regexp.Regexp
	apiKeyPattern   *regexp.Regexp

	// Options
	strictMode     bool // Fail on any validation error
	allowEmptyString bool // Allow empty strings
	maxStringLen   int  // Maximum string length
	maxInt         int64 // Maximum integer value
	minInt         int64 // Minimum integer value
	maxArrayLen    int  // Maximum array length
	sanitization   bool // Enable input sanitization
}

// ValidationResult contains validation results
type ValidationResult struct {
	Valid      bool
	Errors     []string
	Sanitized  bool
	SanitizedValue interface{}
}

// FieldValidator validates a specific field
type FieldValidator struct {
	Name       string
	Required   bool
	Type       string  // string, int, float, bool, array, object, email, url, ip, ipv6, hostname, apikey
	Pattern    string  // Regex pattern for strings
	Min        int64   // Minimum value for numbers
	Max        int64   // Maximum value for numbers
	MinLen     int     // Minimum length for strings/arrays
	MaxLen     int     // Maximum length for strings/arrays
	Allowed    []interface{} // Allowed values (enum)
	Sanitize   bool    // Enable sanitization for this field
}

// Schema represents a validation schema
type Schema struct {
	Fields   []FieldValidator
	Required []string // List of required field names
}

// DefaultOptions returns default validator options
func DefaultOptions() Options {
	return Options{
		StrictMode:       true,
		AllowEmptyString: false,
		MaxStringLen:     10000,
		MaxInt:           9223372036854775807,
		MinInt:           -9223372036854775808,
		MaxArrayLen:      1000,
		Sanitization:     true,
	}
}

// Options contains validator configuration
type Options struct {
	StrictMode       bool
	AllowEmptyString bool
	MaxStringLen     int
	MaxInt           int64
	MinInt           int64
	MaxArrayLen      int
	Sanitization     bool
}

// NewValidator creates a new validator with given options
func NewValidator(opts Options) *Validator {
	v := &Validator{
		strictMode:       opts.StrictMode,
		allowEmptyString: opts.AllowEmptyString,
		maxStringLen:     opts.MaxStringLen,
		maxInt:           opts.MaxInt,
		minInt:           opts.MinInt,
		maxArrayLen:      opts.MaxArrayLen,
		sanitization:     opts.Sanitization,
	}

	// Initialize patterns
	v.emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	v.urlPattern = regexp.MustCompile(`^https?://`)
	v.ipPattern = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	v.ipv6Pattern = regexp.MustCompile(`^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$|^::$`)
	v.hostnamePattern = regexp.MustCompile(`^([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}$`)
	v.apiKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{20,}$`)

	return v
}

// ValidateRequest validates a request against a schema
func (v *Validator) ValidateRequest(c context.Context, ctx *app.RequestContext, schema *Schema) *ValidationResult {
	result := &ValidationResult{
		Valid: true,
		Errors: make([]string, 0),
	}

	if schema == nil {
		return result
	}

	// Bind and validate the request
	var data map[string]interface{}
	if err := ctx.Bind(&data); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Invalid request format: %v", err))
		return result
	}

	// Check required fields
	for _, reqField := range schema.Required {
		if _, exists := data[reqField]; !exists {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Missing required field: %s", reqField))
		}
	}

	// Validate each field
	for _, field := range schema.Fields {
		if err := v.validateField(data, field); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, err.Error())
		}
	}

	return result
}

// ValidateField validates a single field (exported for testing)
func (v *Validator) ValidateField(data map[string]interface{}, field FieldValidator) error {
	return v.validateField(data, field)
}

// ValidateType checks if value matches expected type (exported for testing)
func (v *Validator) ValidateType(value interface{}, expectedType string) error {
	return v.validateType(value, expectedType)
}

// validateField validates a single field
func (v *Validator) validateField(data map[string]interface{}, field FieldValidator) error {
	value, exists := data[field.Name]

	// Check required
	if !exists {
		if field.Required {
			return fmt.Errorf("field %q is required", field.Name)
		}
		return nil // Optional field, not present
	}

	// Check type
	if err := v.validateType(value, field.Type); err != nil {
		return err
	}

	// Validate based on type
	switch field.Type {
	case "string":
		if err := v.validateString(value.(string), field); err != nil {
			return err
		}
	case "int":
		if err := v.validateInt(value, field); err != nil {
			return err
		}
	case "float":
		if err := v.validateFloat(value, field); err != nil {
			return err
		}
	case "bool":
		// Already validated by type check
	case "array":
		if err := v.validateArray(value, field); err != nil {
			return err
		}
	case "object":
		// Nested object validation - skip for now
	case "email":
		if strVal, ok := value.(string); ok {
			if !v.emailPattern.MatchString(strVal) {
				return fmt.Errorf("field %q must be a valid email", field.Name)
			}
		}
	case "url":
		if strVal, ok := value.(string); ok {
			if !v.urlPattern.MatchString(strVal) {
				return fmt.Errorf("field %q must be a valid URL", field.Name)
			}
			// Additional URL validation
			if _, err := url.Parse(strVal); err != nil {
				return fmt.Errorf("field %q must be a parseable URL", field.Name)
			}
		}
	case "ip":
		if strVal, ok := value.(string); ok {
			if net.ParseIP(strVal) == nil {
				return fmt.Errorf("field %q must be a valid IP address", field.Name)
			}
		}
	case "ipv6":
		if strVal, ok := value.(string); ok {
			ip := net.ParseIP(strVal)
			if ip == nil || ip.To4() != nil {
				return fmt.Errorf("field %q must be a valid IPv6 address", field.Name)
			}
		}
	case "hostname":
		if strVal, ok := value.(string); ok {
			if !v.hostnamePattern.MatchString(strVal) {
				return fmt.Errorf("field %q must be a valid hostname", field.Name)
			}
		}
	case "apikey":
		if strVal, ok := value.(string); ok {
			if !v.apiKeyPattern.MatchString(strVal) {
				return fmt.Errorf("field %q must be a valid API key (min 20 chars, alphanumeric)", field.Name)
			}
		}
	}

	// Check allowed values (enum)
	if field.Allowed != nil {
		valid := false
		for _, allowed := range field.Allowed {
			if value == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("field %q must be one of: %v", field.Name, field.Allowed)
		}
	}

	return nil
}

// validateType checks if value matches expected type
func (v *Validator) validateType(value interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field must be a string, got %T", value)
		}
	case "int":
		switch value.(type) {
		case int, int32, int64, float64:
			// OK - float64 is common from JSON unmarshaling
		default:
			return fmt.Errorf("field must be an integer, got %T", value)
		}
	case "float":
		switch value.(type) {
		case float64:
			// OK
		default:
			return fmt.Errorf("field must be a float, got %T", value)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field must be a boolean, got %T", value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("field must be an array, got %T", value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("field must be an object, got %T", value)
		}
	case "email", "url", "ip", "ipv6", "hostname", "apikey":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field must be a string, got %T", value)
		}
	}
	return nil
}

// validateString validates a string field
func (v *Validator) validateString(value string, field FieldValidator) error {
	// Check empty string
	if value == "" {
		if !v.allowEmptyString && field.Required {
			return fmt.Errorf("field %q cannot be empty", field.Name)
		}
		return nil
	}

	// Check length
	if field.MinLen > 0 && len(value) < field.MinLen {
		return fmt.Errorf("field %q must be at least %d characters", field.Name, field.MinLen)
	}
	if field.MaxLen > 0 && len(value) > field.MaxLen {
		return fmt.Errorf("field %q must be at most %d characters", field.Name, field.MaxLen)
	}
	if v.maxStringLen > 0 && len(value) > v.maxStringLen {
		return fmt.Errorf("field %q exceeds maximum length of %d", field.Name, v.maxStringLen)
	}

	// Check pattern
	if field.Pattern != "" {
		pattern := regexp.MustCompile(field.Pattern)
		if !pattern.MatchString(value) {
			return fmt.Errorf("field %q does not match required pattern", field.Name)
		}
	}

	return nil
}

// validateInt validates an integer field
func (v *Validator) validateInt(value interface{}, field FieldValidator) error {
	var intVal int64

	switch v := value.(type) {
	case int:
		intVal = int64(v)
	case int32:
		intVal = int64(v)
	case int64:
		intVal = v
	case float64:
		if v != float64(int64(v)) {
			return fmt.Errorf("field %q must be an integer", field.Name)
		}
		intVal = int64(v)
	default:
		return fmt.Errorf("field must be an integer, got %T", value)
	}

	// Check range
	if field.Min > 0 && intVal < field.Min {
		return fmt.Errorf("field %q must be at least %d", field.Name, field.Min)
	}
	if field.Max > 0 && intVal > field.Max {
		return fmt.Errorf("field %q must be at most %d", field.Name, field.Max)
	}
	if v.minInt > 0 && intVal < v.minInt {
		return fmt.Errorf("field %q exceeds minimum value", field.Name)
	}
	if v.maxInt > 0 && intVal > v.maxInt {
		return fmt.Errorf("field %q exceeds maximum value", field.Name)
	}

	return nil
}

// validateFloat validates a float field
func (v *Validator) validateFloat(value interface{}, field FieldValidator) error {
	if _, ok := value.(float64); !ok {
		return fmt.Errorf("field must be a float, got %T", value)
	}

	// Add range checks if needed
	if field.Min > 0 {
		if value.(float64) < float64(field.Min) {
			return fmt.Errorf("field %q must be at least %d", field.Name, field.Min)
		}
	}
	if field.Max > 0 {
		if value.(float64) > float64(field.Max) {
			return fmt.Errorf("field %q must be at most %d", field.Name, field.Max)
		}
	}

	return nil
}

// validateArray validates an array field
func (v *Validator) validateArray(value interface{}, field FieldValidator) error {
	arr, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("field must be an array, got %T", value)
	}

	// Check length
	if field.MinLen > 0 && len(arr) < field.MinLen {
		return fmt.Errorf("field %q must have at least %d elements", field.Name, field.MinLen)
	}
	if field.MaxLen > 0 && len(arr) > field.MaxLen {
		return fmt.Errorf("field %q must have at most %d elements", field.Name, field.MaxLen)
	}
	if v.maxArrayLen > 0 && len(arr) > v.maxArrayLen {
		return fmt.Errorf("field %q exceeds maximum length of %d", field.Name, v.maxArrayLen)
	}

	return nil
}

// SanitizeInput sanitizes input data
func (v *Validator) SanitizeInput(data interface{}) interface{} {
	if !v.sanitization {
		return data
	}

	switch d := data.(type) {
	case string:
		return v.sanitizeString(d)
	case []interface{}:
		result := make([]interface{}, len(d))
		for i, item := range d {
			result[i] = v.SanitizeInput(item)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range d {
			result[v.sanitizeString(key)] = v.SanitizeInput(value)
		}
		return result
	default:
		return data
	}
}

// sanitizeString sanitizes a string value
func (v *Validator) sanitizeString(input string) string {
	// Trim whitespace
	result := strings.TrimSpace(input)

	// Normalize whitespace
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")

	// Remove potentially dangerous characters (basic XSS prevention)
	// Keep alphanumeric and common safe characters
	result = regexp.MustCompile(`[<>"']`).ReplaceAllString(result, "")

	// Limit length
	if v.maxStringLen > 0 && len(result) > v.maxStringLen {
		result = result[:v.maxStringLen]
	}

	return result
}

// Middleware creates a validation middleware
func Middleware(schema *Schema, opts Options) app.HandlerFunc {
	validator := NewValidator(opts)

	return func(c context.Context, ctx *app.RequestContext) {
		result := validator.ValidateRequest(c, ctx, schema)

		if !result.Valid {
			var requestID string
			if c != nil {
				requestID = logging.GetRequestID(c)
			}

			// Only log if logger is initialized
			if logging.Logger != nil {
				logging.Logger.Warn("Request validation failed",
					logging.WithRequestID(requestID),
					zap.Strings("errors", result.Errors),
				)
			}

			ctx.JSON(consts.StatusBadRequest, map[string]interface{}{
				"error":       "validation failed",
				"errors":      result.Errors,
				"request_id":  requestID,
			})
			return
		}

		// Sanitize input if enabled
		if result.Sanitized && validator.sanitization {
			// Input has been sanitized
		}

		ctx.Next(c)
	}
}

// LLMChatCompletionSchema returns a validation schema for LLM chat completion requests
func LLMChatCompletionSchema() *Schema {
	return &Schema{
		Required: []string{"model", "messages"},
		Fields: []FieldValidator{
			{
				Name:     "model",
				Required: true,
				Type:     "string",
				MinLen:   1,
				MaxLen:   100,
			},
			{
				Name:     "messages",
				Required: true,
				Type:     "array",
				MinLen:   1,
				MaxLen:   100,
			},
			{
				Name:    "max_tokens",
				Type:    "int",
				Min:     1,
				Max:     32000,
			},
			{
				Name:    "temperature",
				Type:    "float",
				Min:     0,
				Max:     2,
			},
			{
				Name:    "top_p",
				Type:    "float",
				Min:     0,
				Max:     1,
			},
			{
				Name:    "stream",
				Type:    "bool",
			},
			{
				Name:    "stop",
				Type:    "array",
				MaxLen:  4,
			},
		},
	}
}

// QueueJobSchema returns a validation schema for queue job requests
func QueueJobSchema() *Schema {
	return &Schema{
		Required: []string{"job_type", "payload"},
		Fields: []FieldValidator{
			{
				Name:     "job_type",
				Required: true,
				Type:     "string",
				MinLen:   1,
				MaxLen:   50,
			},
			{
				Name:     "payload",
				Required: true,
				Type:     "object",
			},
			{
				Name:    "queue",
				Type:    "string",
				MaxLen:  50,
			},
			{
				Name:    "max_retries",
				Type:    "int",
				Min:     0,
				Max:     10,
			},
			{
				Name:    "timeout",
				Type:    "string",
				Pattern: `^\d+[smh]?$`, // Matches "30s", "1m", "2h", etc.
			},
		},
	}
}

// HealthCheckSchema returns a validation schema for health check requests (empty - accepts any request)
func HealthCheckSchema() *Schema {
	return &Schema{
		Required: []string{},
		Fields:   []FieldValidator{},
	}
}

// TenantCreateSchema returns a validation schema for tenant creation requests
func TenantCreateSchema() *Schema {
	return &Schema{
		Required: []string{"name", "email"},
		Fields: []FieldValidator{
			{
				Name:     "name",
				Required: true,
				Type:     "string",
				MinLen:   1,
				MaxLen:   100,
			},
			{
				Name:     "email",
				Required: true,
				Type:     "email",
				MaxLen:   255,
			},
			{
				Name:    "plan",
				Type:    "string",
				Allowed: []interface{}{"free", "basic", "pro", "enterprise"},
			},
		},
	}
}

// APIKeyCreateSchema returns a validation schema for API key creation requests
func APIKeyCreateSchema() *Schema {
	return &Schema{
		Required: []string{"name"},
		Fields: []FieldValidator{
			{
				Name:     "name",
				Required: true,
				Type:     "string",
				MinLen:   1,
				MaxLen:   100,
			},
			{
				Name:    "expires_at",
				Type:    "string",
				Pattern: `^\d{4}-\d{2}-\d{2}$`, // YYYY-MM-DD format
			},
		},
	}
}
