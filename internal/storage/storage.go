package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go-llm-server/internal/config"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/db"
	"go-llm-server/pkg/logger"
	cache "go-llm-server/pkg/redis"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// Storage composes Postgres and Redis for read-through/write-through caching.
type Storage struct {
	DB    *db.Postgres
	Cache *cache.Redis
}

// NewStorage initializes Postgres and Redis from app config and returns a Storage.
func NewStorage(cfg *config.Config) (*Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	pg, err := db.NewPostgres(cfg.Database)
	if err != nil {
		return nil, err
	}

	redisCfg := &cache.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}

	r, err := cache.NewRedis(redisCfg)
	if err != nil {
		pg.Close()
		return nil, err
	}

	return &Storage{DB: pg, Cache: r}, nil
}

// Close releases underlying resources.
func (s *Storage) Close() {
	if s == nil {
		return
	}
	if s.Cache != nil {
		_ = s.Cache.Close()
	}
	if s.DB != nil {
		s.DB.Close()
	}
}

// ---------------- Embedding cache ----------------

// GetEmbedding tries Redis first, then Postgres; on hit from Postgres it backfills Redis.
func (s *Storage) GetEmbedding(ctx context.Context, inputText, modelName string) (*db.EmbeddingRecord, error) {
	if s == nil || s.DB == nil || s.Cache == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	hash := utils.MakeHash(inputText + modelName)
	key := "embedding:" + hash

	var rec db.EmbeddingRecord
	found, err := s.Cache.Get(ctx, key, &rec)
	if err != nil {
		// Log error but continue to Postgres - Redis failure shouldn't break the flow
		logger.Warn("Redis Get failed, falling back to Postgres",
			zap.String("key", key),
			zap.String("model", modelName),
			zap.Error(err))
	} else if found {
		return &rec, nil
	}

	pgRec, err := s.DB.GetEmbedding(ctx, inputText, modelName)
	if err != nil {
		logger.Error("Failed to get embedding from Postgres",
			zap.String("key", key),
			zap.String("model", modelName),
			zap.Error(err))
		return nil, err
	}
	if pgRec != nil {
		if err := s.Cache.Set(ctx, key, pgRec, time.Hour); err != nil {
			// Log cache backfill failure but don't fail the request
			logger.Warn("Failed to backfill Redis cache for embedding",
				zap.String("key", key),
				zap.String("model", modelName),
				zap.Error(err))
		}
	}
	return pgRec, nil
}

// UpsertEmbedding writes to Postgres and updates Redis.
func (s *Storage) UpsertEmbedding(ctx context.Context, inputText, modelName string, embedding []float64) error {
	if s == nil || s.DB == nil || s.Cache == nil {
		return fmt.Errorf("storage not initialized")
	}

	if err := s.DB.UpsertEmbedding(ctx, inputText, modelName, embedding); err != nil {
		logger.Error("Failed to upsert embedding to Postgres",
			zap.String("model", modelName),
			zap.Int("embedding_size", len(embedding)),
			zap.Error(err))
		return err
	}

	hash := utils.MakeHash(inputText + modelName)
	key := "embedding:" + hash
	rec := db.EmbeddingRecord{InputHash: hash, InputText: inputText, ModelName: modelName, Embedding: embedding}
	if err := s.Cache.Set(ctx, key, rec, time.Hour); err != nil {
		// Log cache update failure but don't fail the request since DB write succeeded
		logger.Warn("Failed to update Redis cache for embedding after DB write",
			zap.String("key", key),
			zap.String("model", modelName),
			zap.Error(err))
	}
	return nil
}

// ---------------- LLM cache ----------------

// GetLLM tries Redis first, then Postgres; on hit from Postgres it backfills Redis.
func (s *Storage) GetLLM(ctx context.Context, request, modelName string) (*db.LLMRecord, error) {
	if s == nil || s.DB == nil || s.Cache == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	hash := utils.MakeHash(request)
	key := "llm:" + hash

	var rec db.LLMRecord
	found, err := s.Cache.Get(ctx, key, &rec)
	if err != nil {
		// Log error but continue to Postgres - Redis failure shouldn't break the flow
		logger.Warn("Redis Get failed, falling back to Postgres",
			zap.String("key", key),
			zap.String("model", modelName),
			zap.Error(err))
	} else if found {
		return &rec, nil
	}

	pgRec, err := s.DB.GetLLM(ctx, request)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("Failed to get LLM response from Postgres",
			zap.String("key", key),
			zap.String("model", modelName),
			zap.Error(err))
		return nil, err
	}
	if pgRec != nil {
		if err := s.Cache.Set(ctx, key, pgRec, time.Hour); err != nil {
			// Log cache backfill failure but don't fail the request
			logger.Warn("Failed to backfill Redis cache for LLM",
				zap.String("key", key),
				zap.String("model", modelName),
				zap.Error(err))
		}
	}
	return pgRec, nil
}

// UpsertLLM writes to Postgres and updates Redis.
func (s *Storage) UpsertLLM(ctx context.Context, rec *db.LLMRecord) error {
	if s == nil || s.DB == nil || s.Cache == nil {
		return fmt.Errorf("storage not initialized")
	}
	if rec == nil {
		return fmt.Errorf("LLMRecord cannot be nil")
	}

	if err := s.DB.UpsertLLM(ctx, rec); err != nil {
		logger.Error("Failed to upsert LLM response to Postgres",
			zap.String("model", rec.ModelName),
			zap.Error(err))
		return err
	}

	// 如果 RequestHash 为空，需要构建哈希
	hash := rec.RequestHash
	if hash == "" {
		requestStr := string(rec.Request)
		hash = utils.MakeHash(requestStr)
		rec.RequestHash = hash
	}
	key := "llm:" + hash

	if err := s.Cache.Set(ctx, key, rec, time.Hour); err != nil {
		// Log cache update failure but don't fail the request since DB write succeeded
		logger.Warn("Failed to update Redis cache for LLM after DB write",
			zap.String("key", key),
			zap.String("model", rec.ModelName),
			zap.Error(err))
	}
	return nil
}
