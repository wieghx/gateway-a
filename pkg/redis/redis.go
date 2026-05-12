package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/gu/gateway-a/config"
	"github.com/redis/go-redis/v9"
)

// Client is the Redis client instance
var Client *redis.Client

// InitClient initializes the Redis client
func InitClient(cfg *config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection with PING
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	Client = client
	return client, nil
}

// IsConnected checks if Redis is connected
func IsConnected() bool {
	if Client == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Client.Ping(ctx).Err()
	return err == nil
}

// HealthCheck performs a health check on Redis connection
func HealthCheck() error {
	if Client == nil {
		return fmt.Errorf("Redis client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis ping failed: %w", err)
	}

	return nil
}
