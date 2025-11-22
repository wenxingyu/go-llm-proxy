package db

import (
	"context"
	"fmt"
	"go-llm-server/internal/utils"
	"time"

	"go-llm-server/internal/config"
	"go-llm-server/pkg/logger"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// SQL query constants for prepared statement caching
const (
	sqlUpsertEmbedding = `
		INSERT INTO embedding_cache (input_hash, input_text, model_name, embedding)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (input_hash, model_name)
		DO UPDATE SET embedding = EXCLUDED.embedding, updated_at = NOW()`

	sqlUpsertLLM = `
		INSERT INTO llm_cache (request_hash, request_id, request, model_name, temperature, max_tokens, response, total_tokens, prompt_tokens, completion_tokens, start_time, end_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (request_hash, model_name)
		DO UPDATE SET request_id = EXCLUDED.request_id, response = EXCLUDED.response, total_tokens = EXCLUDED.total_tokens, prompt_tokens = EXCLUDED.prompt_tokens, completion_tokens = EXCLUDED.completion_tokens, start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time, updated_at = NOW()`

	sqlGetEmbedding = `
		SELECT id, input_hash, input_text, model_name, embedding, created_at, updated_at, expire_at
		FROM embedding_cache
		WHERE input_hash = $1 AND model_name = $2`

	sqlGetLLM = `
		SELECT id, request_hash, request_id, request, model_name, temperature, max_tokens, response, total_tokens, prompt_tokens, completion_tokens, start_time, end_time, created_at, updated_at, expire_at
		FROM llm_cache
		WHERE request_hash = $1`

	sqlListEmbeddings = `
		SELECT id, input_hash, input_text, model_name, embedding, created_at, updated_at, expire_at
		FROM embedding_cache
		WHERE model_name = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	sqlListLLMs = `
		SELECT id, request_hash, request_id, request, model_name, temperature, max_tokens, response, total_tokens, prompt_tokens, completion_tokens, start_time, end_time, created_at, updated_at, expire_at
		FROM llm_cache
		WHERE model_name = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	sqlCountEmbeddings = `SELECT COUNT(*) FROM embedding_cache WHERE model_name = $1`
	sqlCountLLMs       = `SELECT COUNT(*) FROM llm_cache WHERE model_name = $1`
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

	// Enable prepared statement caching for better performance
	// pgx v5 enables this by default, but we explicitly set it for clarity
	// Statement cache size per connection (default is 512)
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.MinConns = 2 // Keep at least 2 connections with prepared statements

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

// createTables ensures required tables exist in public schema
func (p *Postgres) createTables(ctx context.Context) error {
	if p == nil || p.Pool == nil {
		return fmt.Errorf("postgres pool is not initialized")
	}
	// Create request_cache
	if _, err := p.Pool.Exec(ctx, ddl); err != nil {
		logger.Error("failed to create table public.request_cache", zap.Error(err))
		return err
	}
	return nil
}

func (p *Postgres) UpsertEmbedding(ctx context.Context, inputText, modelName string, embedding []float64) error {
	hash := utils.MakeHash(inputText + modelName)

	_, err := p.Pool.Exec(ctx, sqlUpsertEmbedding, hash, inputText, modelName, embedding)
	if err != nil {
		return err
	}

	return nil
}

func (p *Postgres) UpsertLLM(ctx context.Context, rec *LLMRecord) error {
	if rec == nil {
		return fmt.Errorf("LLMRecord cannot be nil")
	}

	// 仅使用 Request 计算哈希
	requestStr := string(rec.Request)
	hash := utils.MakeHash(requestStr)
	rec.RequestHash = hash

	_, err := p.Pool.Exec(ctx, sqlUpsertLLM, hash, rec.RequestID, rec.Request, rec.ModelName, rec.Temperature, rec.MaxTokens, rec.Response, rec.TotalTokens, rec.PromptTokens, rec.CompletionTokens, rec.StartTime, rec.EndTime)
	if err != nil {
		return err
	}
	return nil
}

// GetEmbedding retrieves an embedding record by input text and model name
func (p *Postgres) GetEmbedding(ctx context.Context, inputText, modelName string) (*EmbeddingRecord, error) {
	hash := utils.MakeHash(inputText + modelName)

	var record EmbeddingRecord
	err := p.Pool.QueryRow(ctx, sqlGetEmbedding, hash, modelName).Scan(
		&record.ID, &record.InputHash, &record.InputText, &record.ModelName,
		&record.Embedding, &record.CreatedAt, &record.UpdatedAt, &record.ExpireAt,
	)

	if err != nil {
		return nil, err
	}

	return &record, nil
}

// GetLLM retrieves an LLM record by prompt and parameters
func (p *Postgres) GetLLM(ctx context.Context, request string) (*LLMRecord, error) {
	hash := utils.MakeHash(request)

	var record LLMRecord
	err := p.Pool.QueryRow(ctx, sqlGetLLM, hash).Scan(
		&record.ID, &record.RequestHash, &record.RequestID, &record.Request, &record.ModelName,
		&record.Temperature, &record.MaxTokens, &record.Response, &record.TotalTokens,
		&record.PromptTokens, &record.CompletionTokens,
		&record.StartTime, &record.EndTime,
		&record.CreatedAt, &record.UpdatedAt, &record.ExpireAt,
	)

	if err != nil {
		return nil, err
	}

	return &record, nil
}

// ListEmbeddings retrieves embedding records with pagination
func (p *Postgres) ListEmbeddings(ctx context.Context, modelName string, limit, offset int) ([]EmbeddingRecord, error) {
	rows, err := p.Pool.Query(ctx, sqlListEmbeddings, modelName, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []EmbeddingRecord
	for rows.Next() {
		var record EmbeddingRecord
		err := rows.Scan(
			&record.ID, &record.InputHash, &record.InputText, &record.ModelName,
			&record.Embedding, &record.CreatedAt, &record.UpdatedAt, &record.ExpireAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, rows.Err()
}

// ListLLMs retrieves LLM records with pagination
func (p *Postgres) ListLLMs(ctx context.Context, modelName string, limit, offset int) ([]LLMRecord, error) {
	rows, err := p.Pool.Query(ctx, sqlListLLMs, modelName, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []LLMRecord
	for rows.Next() {
		var record LLMRecord
		err := rows.Scan(
			&record.ID, &record.RequestHash, &record.RequestID, &record.Request, &record.ModelName,
			&record.Temperature, &record.MaxTokens, &record.Response, &record.TotalTokens,
			&record.PromptTokens, &record.CompletionTokens,
			&record.StartTime, &record.EndTime,
			&record.CreatedAt, &record.UpdatedAt, &record.ExpireAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, rows.Err()
}

// CountEmbeddings returns the total count of embedding records for a model
func (p *Postgres) CountEmbeddings(ctx context.Context, modelName string) (int, error) {
	var count int
	err := p.Pool.QueryRow(ctx, sqlCountEmbeddings, modelName).Scan(&count)
	return count, err
}

// CountLLMs returns the total count of LLM records for a model
func (p *Postgres) CountLLMs(ctx context.Context, modelName string) (int, error) {
	var count int
	err := p.Pool.QueryRow(ctx, sqlCountLLMs, modelName).Scan(&count)
	return count, err
}
