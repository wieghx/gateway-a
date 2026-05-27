package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wieghx/gateway-a/config"
	"github.com/wieghx/gateway-a/internal/handler"
	"github.com/wieghx/gateway-a/internal/logging"
	"github.com/wieghx/gateway-a/internal/server"
	"github.com/wieghx/gateway-a/pkg/database"
	"github.com/wieghx/gateway-a/pkg/redis"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Parse config file path
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logLevel, err := parseLogLevel(cfg.Logging.Level)
	if err != nil {
		log.Fatalf("Failed to parse log level: %v", err)
	}
	logger, err := logging.InitLogger(logLevel)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	logging.Logger = logger
	defer logger.Sync()

	log.Println("Starting AI Gateway...")
	logging.Logger.Info("Gateway starting",
		logging.WithRequestID("init"),
		zap.String("config_file", *configPath),
	)

	// Initialize Redis client
	redisClient, err := redis.InitClient(&cfg.Redis)
	if err != nil {
		logging.Logger.Warn("Redis not available, rate limiting disabled",
			zap.Error(err),
		)
	} else {
		logging.Logger.Info("Redis connected",
			zap.String("host", cfg.Redis.Host),
			zap.String("port", cfg.Redis.Port),
		)
	}

	// Initialize database
	db, err := database.InitDB(&cfg.Database)
	if err != nil {
		logging.Logger.Warn("PostgreSQL not available, some features disabled",
			zap.Error(err),
		)
	} else {
		logging.Logger.Info("Database connected",
			zap.String("host", cfg.Database.Host),
			zap.String("port", cfg.Database.Port),
			zap.String("dbname", cfg.Database.DBName),
		)
		// Wire DB to handlers (fixes auth/quota)
		handler.SetDB(db)
	}

	// Initialize server with loaded config for proper LLM settings etc.
	srv := server.NewServer(cfg)

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdown
		logging.Logger.Info("Shutting down...")

		// Gracefully shutdown server
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logging.Logger.Error("Server shutdown error",
				zap.Error(err),
			)
		}

		// Close database connection
		if db != nil {
			sqlDB, err := db.DB()
			if err != nil {
				logging.Logger.Error("Failed to get database instance for shutdown",
					zap.Error(err),
				)
			} else {
				sqlDB.Close()
				logging.Logger.Info("Database connection closed")
			}
		}

		// Close Redis connection
		if redisClient != nil {
			redisClient.Close()
			logging.Logger.Info("Redis connection closed")
		}

		logging.Logger.Info("Gateway shutdown complete")
	}()

	if err := srv.Start(); err != nil {
		logging.Logger.Error("Server error",
			zap.Error(err),
		)
		log.Fatalf("Failed to start server: %v", err)
	}
}

func parseLogLevel(level string) (zapcore.Level, error) {
	var logLevel zapcore.Level
	err := logLevel.UnmarshalText([]byte(strings.ToLower(level)))
	return logLevel, err
}
