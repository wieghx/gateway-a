package logging

import (
	"context"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the structured logger instance
var Logger *zap.Logger

// RequestIDKey is the context key for request ID
type RequestIDKey struct{}

// InitLogger initializes the structured logger with request ID support
func InitLogger(level zapcore.Level) (*zap.Logger, error) {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    "",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return logger, nil
}

// GetRequestID extracts request ID from context
func GetRequestID(ctx context.Context) string {
	if rid, ok := ctx.Value(RequestIDKey{}).(string); ok {
		return rid
	}
	return ""
}

// SetRequestID sets request ID in context
func SetRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey{}, requestID)
}

// WithRequestID returns a logger field for request ID
func WithRequestID(requestID string) zap.Field {
	return zap.String("request_id", requestID)
}

// SanitizeString removes control characters from strings for logging
func SanitizeString(s string) string {
	var builder strings.Builder
	for _, r := range s {
		if r >= 32 && r != 127 {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
