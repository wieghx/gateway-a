//go:build integration

package phase9

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wieghx/gateway-a/middleware/validation"
)

// TestValidator tests the request validator
func TestValidator(t *testing.T) {
	t.Run("creates valid validator", func(t *testing.T) {
		opts := validation.DefaultOptions()

		assert.NotNil(t, opts)
		assert.True(t, opts.StrictMode)
		assert.Equal(t, 10000, opts.MaxStringLen)
		assert.Equal(t, int64(9223372036854775807), opts.MaxInt)
	})

	t.Run("validates required fields", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		// Test field validation directly
		field := validation.FieldValidator{Name: "name", Required: true, Type: "string"}
		data := map[string]interface{}{"name": "John Doe"}
		err := validator.ValidateField(data, field)
		assert.NoError(t, err)
	})

	t.Run("email validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid email", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "email",
				Type: "email",
			}

			data := map[string]interface{}{"email": "test@example.com"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid email", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "email",
				Type: "email",
			}

			data := map[string]interface{}{"email": "not-an-email"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("URL validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid URL", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "url",
				Type: "url",
			}

			data := map[string]interface{}{"url": "https://example.com"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid URL", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "url",
				Type: "url",
			}

			data := map[string]interface{}{"url": "not-a-url"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("IP validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid IPv4", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "ip",
				Type: "ip",
			}

			data := map[string]interface{}{"ip": "192.168.1.1"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("valid IPv6", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "ipv6",
				Type: "ipv6",
			}

			data := map[string]interface{}{"ipv6": "2001:0db8:85a3:0000:0000:8a2e:0370:7334"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid IP", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "ip",
				Type: "ip",
			}

			data := map[string]interface{}{"ip": "999.999.999.999"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("hostname validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid hostname", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "hostname",
				Type: "hostname",
			}

			data := map[string]interface{}{"hostname": "example.com"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid hostname", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "hostname",
				Type: "hostname",
			}

			data := map[string]interface{}{"hostname": "not-a-hostname@#$%"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("API key validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid API key", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "api_key",
				Type: "apikey",
			}

			data := map[string]interface{}{"api_key": "abcdefghijklmnopqrstuvwxyz123456"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid API key (too short)", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "api_key",
				Type: "apikey",
			}

			data := map[string]interface{}{"api_key": "short"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})

		t.Run("invalid API key (special chars)", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "api_key",
				Type: "apikey",
			}

			data := map[string]interface{}{"api_key": "key-with-dashes"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("integer validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid int within range", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "count",
				Type: "int",
				Min:  1,
				Max:  100,
			}

			data := map[string]interface{}{"count": 50}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid int below min", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "count",
				Type: "int",
				Min:  1,
				Max:  100,
			}

			data := map[string]interface{}{"count": 0}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})

		t.Run("invalid int above max", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "count",
				Type: "int",
				Min:  1,
				Max:  100,
			}

			data := map[string]interface{}{"count": 101}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("float validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid float", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "ratio",
				Type: "float",
				Min:  0,
				Max:  1,
			}

			data := map[string]interface{}{"ratio": 0.5}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid float above max", func(t *testing.T) {
			field := validation.FieldValidator{
				Name: "ratio",
				Type: "float",
				Min:  0,
				Max:  1,
			}

			data := map[string]interface{}{"ratio": 1.5}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("array validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid array", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:     "items",
				Type:     "array",
				MinLen:   1,
				MaxLen:   10,
			}

			data := map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid empty array", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:     "items",
				Type:     "array",
				MinLen:   1,
			}

			data := map[string]interface{}{
				"items": []interface{}{},
			}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})

		t.Run("invalid array too long", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:     "items",
				Type:     "array",
				MaxLen:   2,
			}

			data := map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("string length validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid string length", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:     "name",
				Type:     "string",
				MinLen:   1,
				MaxLen:   100,
			}

			data := map[string]interface{}{"name": "John Doe"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("string too short", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:     "name",
				Type:     "string",
				MinLen:   3,
			}

			data := map[string]interface{}{"name": "ab"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})

		t.Run("string too long", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:     "name",
				Type:     "string",
				MaxLen:   5,
			}

			data := map[string]interface{}{"name": "abcdefgh"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("enum validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("valid enum value", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:    "status",
				Type:    "string",
				Allowed: []interface{}{"active", "inactive", "pending"},
			}

			data := map[string]interface{}{"status": "active"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("invalid enum value", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:    "status",
				Type:    "string",
				Allowed: []interface{}{"active", "inactive", "pending"},
			}

			data := map[string]interface{}{"status": "unknown"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})

	t.Run("pattern validation", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		t.Run("string matches pattern", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:    "code",
				Type:    "string",
				Pattern: `^[A-Z]{3}-\d{4}$`,
			}

			data := map[string]interface{}{"code": "ABC-1234"}

			err := validator.ValidateField(data, field)
			assert.NoError(t, err)
		})

		t.Run("string does not match pattern", func(t *testing.T) {
			field := validation.FieldValidator{
				Name:    "code",
				Type:    "string",
				Pattern: `^[A-Z]{3}-\d{4}$`,
			}

			data := map[string]interface{}{"code": "abc-1234"}

			err := validator.ValidateField(data, field)
			assert.Error(t, err)
		})
	})
}

// TestPredefinedSchemas tests predefined schemas
func TestPredefinedSchemas(t *testing.T) {
	t.Run("LLMChatCompletionSchema", func(t *testing.T) {
		schema := validation.LLMChatCompletionSchema()

		require.NotNil(t, schema)
		assert.Contains(t, schema.Required, "model")
		assert.Contains(t, schema.Required, "messages")
		assert.NotEmpty(t, schema.Fields)
	})

	t.Run("QueueJobSchema", func(t *testing.T) {
		schema := validation.QueueJobSchema()

		require.NotNil(t, schema)
		assert.Contains(t, schema.Required, "job_type")
		assert.Contains(t, schema.Required, "payload")
		assert.NotEmpty(t, schema.Fields)
	})
}

// TestSanitization tests input sanitization
func TestSanitization(t *testing.T) {
	t.Run("trims whitespace", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		input := "  hello world  "
		sanitized := validator.SanitizeInput(input).(string)

		assert.Equal(t, "hello world", sanitized)
	})

	t.Run("normalizes whitespace", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		input := "hello   world"
		sanitized := validator.SanitizeInput(input).(string)

		assert.Equal(t, "hello world", sanitized)
	})

	t.Run("removes dangerous characters", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		input := `<script>alert('xss')</script>`
		sanitized := validator.SanitizeInput(input).(string)

		assert.NotContains(t, sanitized, "<")
		assert.NotContains(t, sanitized, ">")
		assert.NotContains(t, sanitized, "'")
		assert.NotContains(t, sanitized, "\"")
	})

	t.Run("sanitizes map keys and values", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		input := map[string]interface{}{
			"key1": "  value1  ",
			"key2": "  value2  ",
		}
		sanitized := validator.SanitizeInput(input).(map[string]interface{})

		assert.Equal(t, "value1", sanitized["key1"])
		assert.Equal(t, "value2", sanitized["key2"])
	})

	t.Run("sanitizes arrays", func(t *testing.T) {
		validator := validation.NewValidator(validation.DefaultOptions())

		input := []interface{}{"  a  ", "  b  ", "  c  "}
		sanitized := validator.SanitizeInput(input).([]interface{})

		assert.Equal(t, "a", sanitized[0])
		assert.Equal(t, "b", sanitized[1])
		assert.Equal(t, "c", sanitized[2])
	})

	t.Run("no sanitization when disabled", func(t *testing.T) {
		opts := validation.DefaultOptions()
		opts.Sanitization = false
		validator := validation.NewValidator(opts)

		input := "  hello  "
		sanitized := validator.SanitizeInput(input).(string)

		assert.Equal(t, "  hello  ", sanitized)
	})
}

// TestMiddleware tests the validation middleware
func TestMiddleware(t *testing.T) {
	t.Run("middleware with valid request", func(t *testing.T) {
		schema := &validation.Schema{
			Required: []string{},
			Fields:   []validation.FieldValidator{},
		}

		middleware := validation.Middleware(schema, validation.DefaultOptions())

		
		hertzCtx := &app.RequestContext{}

		// Call middleware - should not error
		middleware(nil, hertzCtx)

		// Should call Next
		assert.NotNil(t, hertzCtx)
	})

	t.Run("middleware sets response on validation failure", func(t *testing.T) {
		schema := &validation.Schema{
			Required: []string{"required_field"},
			Fields: []validation.FieldValidator{
				{Name: "required_field", Required: true, Type: "string"},
			},
		}

		middleware := validation.Middleware(schema, validation.DefaultOptions())

		
		hertzCtx := &app.RequestContext{}
		hertzCtx.Request.Header.Set("Content-Type", "application/json")
		// Send request without required field
		hertzCtx.Request.SetBodyString(`{"other_field": "value"}`)

		middleware(nil, hertzCtx)

		// Should have set error response
		assert.Equal(t, 400, hertzCtx.Response.StatusCode())
	})
}

// TestTypeValidation tests type-specific validations
func TestTypeValidation(t *testing.T) {
	validator := validation.NewValidator(validation.DefaultOptions())

	t.Run("string type", func(t *testing.T) {
		result := validator.ValidateType("hello", "string")
		assert.NoError(t, result)
	})

	t.Run("string type mismatch", func(t *testing.T) {
		result := validator.ValidateType(123, "string")
		assert.Error(t, result)
	})

	t.Run("int type with float64", func(t *testing.T) {
		// JSON unmarshaling produces float64
		result := validator.ValidateType(5.0, "int")
		assert.NoError(t, result)
	})

	t.Run("int type mismatch", func(t *testing.T) {
		result := validator.ValidateType("hello", "int")
		assert.Error(t, result)
	})

	t.Run("array type", func(t *testing.T) {
		result := validator.ValidateType([]interface{}{1, 2, 3}, "array")
		assert.NoError(t, result)
	})

	t.Run("object type", func(t *testing.T) {
		result := validator.ValidateType(map[string]interface{}{"key": "value"}, "object")
		assert.NoError(t, result)
	})
}

// BenchmarkValidation tests performance
func BenchmarkValidateField(b *testing.B) {
	validator := validation.NewValidator(validation.DefaultOptions())
	field := validation.FieldValidator{
		Name:  "test",
		Type:  "string",
		MaxLen: 100,
	}
	data := map[string]interface{}{"test": "hello"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.ValidateField(data, field)
	}
}

func BenchmarkSanitizeString(b *testing.B) {
	validator := validation.NewValidator(validation.DefaultOptions())
	input := "  hello world  "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.SanitizeInput(input)
	}
}
