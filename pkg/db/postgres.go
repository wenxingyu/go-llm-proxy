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

	return &Postgres{Pool: pool}, nil
}

// Close closes the pool
func (p *Postgres) Close() {
	if p != nil && p.Pool != nil {
		p.Pool.Close()
	}
}

// InsertLLMLog inserts a row into the logs table with id, query, response, model_name, create_at
// Table example schema:
// CREATE TABLE IF NOT EXISTS llm_logs (
//
//	id TEXT PRIMARY KEY,
//	query TEXT NOT NULL,
//	response TEXT NOT NULL,
//	model_name TEXT NOT NULL,
//	create_at TIMESTAMP NOT NULL DEFAULT NOW()
//
// );
func (p *Postgres) InsertLLMLog(ctx context.Context, id string, query string, response string, modelName string, createAt time.Time) error {
	if p == nil || p.Pool == nil {
		return fmt.Errorf("postgres pool is not initialized")
	}

	const stmt = `
INSERT INTO llm_logs (id, query, response, model_name, create_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO NOTHING;
`

	_, err := p.Pool.Exec(ctx, stmt, id, query, response, modelName, createAt)
	if err != nil {
		logger.Error("failed to insert llm log", zap.Error(err))
		return err
	}
	return nil
}
