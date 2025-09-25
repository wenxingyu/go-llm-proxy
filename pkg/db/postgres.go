package db

import (
	"context"
	"fmt"
	"time"

	"go-llm-server/internal/config"
	"go-llm-server/pkg/logger"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Postgres wraps a pgx connection pool
type Postgres struct {
	Pool *pgxpool.Pool
}

// NewPostgres creates a new pgx pool using DatabaseConfig
func NewPostgres(cfg config.DatabaseConfig) (*Postgres, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, cfg.SSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	// Pool tuning
	if cfg.MaxOpenConns > 0 {
		poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	}
	// pgxpool does not have MaxIdleConns directly; we can keep it for parity
	// Conn lifetime
	if cfg.ConnMaxLifetime > 0 {
		poolConfig.MaxConnLifetime = time.Duration(cfg.ConnMaxLifetime) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}

	// Verify connection with a ping
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	logger.Info("postgres connected",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("db", cfg.DBName),
	)

	pg := &Postgres{Pool: pool}

	// Ensure required tables exist
	if err := pg.createTables(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return pg, nil
}

// Close closes the pool
func (p *Postgres) Close() {
	if p != nil && p.Pool != nil {
		p.Pool.Close()
	}
}

type RequestCache struct {
	ID        int    `json:"id"`
	Key       string `json:"key"`
	Request   string `json:"request"`
	ModelName string `json:"model_name"`
	Response  string `json:"response"`
	CreatedAt string `json:"created_at"`
}

// createTables ensures required tables exist in public schema
func (p *Postgres) createTables(ctx context.Context) error {
	if p == nil || p.Pool == nil {
		return fmt.Errorf("postgres pool is not initialized")
	}

	const createRequestCache = `
CREATE TABLE IF NOT EXISTS public.request_cache (
    id SERIAL PRIMARY KEY,
	key TEXT NOT NULL,
    request TEXT NOT NULL,
    model_name TEXT NOT NULL,
    response TEXT NOT NULL,
    create_at TIMESTAMP NOT NULL DEFAULT NOW()
);
`
	// Create request_cache
	if _, err := p.Pool.Exec(ctx, createRequestCache); err != nil {
		logger.Error("failed to create table public.request_cache", zap.Error(err))
		return err
	}

	return nil
}

func (p *Postgres) InsertRequestCache(ctx context.Context, log *RequestCache) error {
	if p == nil || p.Pool == nil {
		return fmt.Errorf("postgres pool is not initialized")
	}
	const stmt = `
INSERT INTO public.request_cache (key, request, model_name, response)
VALUES ($1, $2, $3, $4)
RETURNING id, key;
`

	var id int
	var key string
	err := p.Pool.QueryRow(ctx, stmt, log.Key, log.Request, log.ModelName, log.Response).Scan(&id, &key)
	if err != nil {
		logger.Error("failed to insert llm log", zap.Error(err))
		return err
	}

	// 更新结构体中的 ID/Key
	log.ID = id
	log.Key = key
	// 如果需要，调用方可以自行计算/使用 key；这里不再暴露在结构体上
	return nil
}
