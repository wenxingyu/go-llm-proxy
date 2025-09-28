package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	client *redis.Client
}

type Config struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// NewRedis creates a new Redis client with the given configuration
func NewRedis(config *Config) (*Redis, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config cannot be nil")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to client: %w", err)
	}

	return &Redis{
		client: rdb,
	}, nil
}

// NewRedisFromConfig creates a new Redis client from the main application config
func NewRedisFromConfig(config interface{}) (*Redis, error) {
	// This function can be used when you have access to the main config
	// For now, it's a placeholder that can be implemented based on your needs
	return nil, fmt.Errorf("not implemented - use NewRedis with RedisConfig directly")
}

func (r *Redis) Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	val, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	err = json.Unmarshal([]byte(val), dest)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *Redis) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// Close closes the Redis connection
func (r *Redis) Close() error {
	return r.client.Close()
}
