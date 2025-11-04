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
	Addr         string `yaml:"addr"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`      // Maximum number of socket connections
	MinIdleConns int    `yaml:"min_idle_conns"` // Minimum number of idle connections
}

// NewRedis creates a new Redis client with the given configuration
func NewRedis(config *Config) (*Redis, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config cannot be nil")
	}

	// Set default pool size if not configured
	poolSize := config.PoolSize
	if poolSize <= 0 {
		poolSize = 10 // Default pool size
	}

	// Set default min idle connections if not configured
	minIdleConns := config.MinIdleConns
	if minIdleConns <= 0 {
		minIdleConns = 5 // Default min idle connections
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
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
