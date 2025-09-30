package storage

import (
	"context"
	"fmt"
	"time"

	"go-llm-server/internal/config"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/db"
	cache "go-llm-server/pkg/redis"
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
		return nil, err
	}
	if found {
		return &rec, nil
	}

	pgRec, err := s.DB.GetEmbedding(ctx, inputText, modelName)
	if err != nil {
		return nil, err
	}
	if pgRec != nil {
		_ = s.Cache.Set(ctx, key, pgRec, time.Hour)
	}
	return pgRec, nil
}

// UpsertEmbedding writes to Postgres and updates Redis.
func (s *Storage) UpsertEmbedding(ctx context.Context, inputText, modelName string, embedding []float64) error {
	if s == nil || s.DB == nil || s.Cache == nil {
		return fmt.Errorf("storage not initialized")
	}

	if err := s.DB.UpsertEmbedding(ctx, inputText, modelName, embedding); err != nil {
		return err
	}

	hash := utils.MakeHash(inputText + modelName)
	key := "embedding:" + hash
	rec := db.EmbeddingRecord{InputHash: hash, InputText: inputText, ModelName: modelName, Embedding: embedding}
	return s.Cache.Set(ctx, key, rec, time.Hour)
}

// ---------------- LLM cache ----------------

// GetLLM tries Redis first, then Postgres; on hit from Postgres it backfills Redis.
func (s *Storage) GetLLM(ctx context.Context, prompt, modelName string, temperature float32, maxTokens int) (*db.LLMRecord, error) {
	if s == nil || s.DB == nil || s.Cache == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	hash := utils.MakeHash(fmt.Sprintf("%s|%s|%f|%d", prompt, modelName, temperature, maxTokens))
	key := "llm:" + hash

	var rec db.LLMRecord
	found, err := s.Cache.Get(ctx, key, &rec)
	if err != nil {
		return nil, err
	}
	if found {
		return &rec, nil
	}

	pgRec, err := s.DB.GetLLM(ctx, prompt, modelName, temperature, maxTokens)
	if err != nil {
		return nil, err
	}
	if pgRec != nil {
		_ = s.Cache.Set(ctx, key, pgRec, time.Hour)
	}
	return pgRec, nil
}

// UpsertLLM writes to Postgres and updates Redis.
func (s *Storage) UpsertLLM(ctx context.Context, prompt, modelName string, temperature float32, maxTokens int, response string, tokensUsed int) error {
	if s == nil || s.DB == nil || s.Cache == nil {
		return fmt.Errorf("storage not initialized")
	}

	if err := s.DB.UpsertLLM(ctx, prompt, modelName, temperature, maxTokens, response, tokensUsed); err != nil {
		return err
	}

	hash := utils.MakeHash(fmt.Sprintf("%s|%s|%f|%d", prompt, modelName, temperature, maxTokens))
	key := "llm:" + hash
	rec := db.LLMRecord{RequestHash: hash, Prompt: prompt, ModelName: modelName, Response: response}
	// include optional fields when available
	t := temperature
	m := maxTokens
	tu := tokensUsed
	rec.Temperature = &t
	rec.MaxTokens = &m
	rec.TokensUsed = &tu
	return s.Cache.Set(ctx, key, rec, time.Hour)
}
