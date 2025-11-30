package storage

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"go-llm-server/internal/config"
	"go-llm-server/internal/utils"
	"go-llm-server/pkg/db"

	redisv9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProvider = "test-provider.local"

func newStorageEmbeddingRecord(inputText, modelName string, embedding []float64) *db.EmbeddingRecord {
	return &db.EmbeddingRecord{
		InputText: inputText,
		ModelName: modelName,
		Provider:  testProvider,
		Embedding: embedding,
	}
}

// Note: These tests require real services reachable at the following addresses,
// matching existing Redis and Postgres integration tests in this repo.
// Postgres: 192.168.70.128:5432 (db: postgres_test, user: postgres, password: postgres_password)
// Redis:    192.168.70.128:6379 (password: myredissecret, db: 0)

func setupTestStorage(t *testing.T) *Storage {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Host:            os.Getenv("DB_HOST"),
			Port:            5432,
			User:            os.Getenv("DB_USER"),
			Password:        os.Getenv("DB_PASSWORD"),
			DBName:          os.Getenv("DB_NAME"),
			SSLMode:         "disable",
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 300,
		},
		Redis: config.RedisConfig{
			Addr:     os.Getenv("REDIS_ADDR"),
			Password: "myredissecret",
			DB:       0,
		},
	}

	s, err := NewStorage(cfg)
	if err != nil {
		t.Skipf("skipping storage tests: %v", err)
	}
	return s
}

func newRawRedis() *redisv9.Client {
	return redisv9.NewClient(&redisv9.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: "myredissecret",
		DB:       0,
	})
}

func TestStorage_EmbeddingFlow(t *testing.T) {
	s := setupTestStorage(t)
	defer s.Close()

	ctx := context.Background()
	inputText := "test_storage_embedding_flow"
	modelName := "text-embedding-ada-002"
	embedding := []float64{0.1, 0.2, 0.3, 0.4}

	// Upsert into DB (and cache)
	require.NoError(t, s.UpsertEmbedding(ctx, newStorageEmbeddingRecord(inputText, modelName, embedding)))

	// Read back (should hit Redis immediately)
	rec, err := s.GetEmbedding(ctx, inputText, modelName, testProvider)
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, inputText, rec.InputText)
	assert.Equal(t, modelName, rec.ModelName)
	assert.Equal(t, embedding, rec.Embedding)

	// Verify value present in Redis by reading the exact key via raw client
	key := "embedding:" + utils.MakeEmbeddingCacheKey(inputText, modelName, testProvider)
	rdb := newRawRedis()
	defer rdb.Close()
	val, err := rdb.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.NotEmpty(t, val)
}

func TestStorage_GetEmbedding_ReadThrough(t *testing.T) {
	s := setupTestStorage(t)
	defer s.Close()

	ctx := context.Background()
	inputText := "test_storage_embedding_readthrough"
	modelName := "text-embedding-ada-002"
	embedding := []float64{0.5, 0.6, 0.7}

	// Ensure DB has the record, but remove from Redis to test read-through
	require.NoError(t, s.UpsertEmbedding(ctx, newStorageEmbeddingRecord(inputText, modelName, embedding)))

	key := "embedding:" + utils.MakeEmbeddingCacheKey(inputText, modelName, testProvider)
	rdb := newRawRedis()
	defer rdb.Close()
	_ = rdb.Del(ctx, key).Err()

	// First Get should fetch from DB and backfill Redis
	start := time.Now()
	rec, err := s.GetEmbedding(ctx, inputText, modelName, testProvider)
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, embedding, rec.Embedding)
	firstLatency := time.Since(start)

	// Second Get should be faster (cache hit)
	start = time.Now()
	_, err = s.GetEmbedding(ctx, inputText, modelName, testProvider)
	require.NoError(t, err)
	secondLatency := time.Since(start)

	// Not strictly deterministic, but usually cache hit is faster
	assert.LessOrEqual(t, secondLatency, firstLatency*2)
}

func TestStorage_LLMFlow(t *testing.T) {
	s := setupTestStorage(t)
	defer s.Close()

	ctx := context.Background()
	prompt := "What is Go?"
	modelName := "gpt-4"
	temperature := float32(0.7)
	maxTokens := 256
	response := `{"answer":"Go is a programming language."}`
	totalTokens := 42
	promptTokens := 10
	completionTokens := 32

	// 将 prompt 和 response 转换为 JSON 格式
	promptJSON, err := json.Marshal(prompt)
	require.NoError(t, err)
	responseJSON := json.RawMessage(response)

	// Upsert into DB (and cache)
	llmRecord := &db.LLMRecord{
		Request:          json.RawMessage(promptJSON),
		ModelName:        modelName,
		Temperature:      &temperature,
		MaxTokens:        &maxTokens,
		Response:         responseJSON,
		TotalTokens:      &totalTokens,
		PromptTokens:     &promptTokens,
		CompletionTokens: &completionTokens,
	}
	require.NoError(t, s.UpsertLLM(ctx, llmRecord))

	// Read back
	rec, err := s.GetLLM(ctx, string(promptJSON), modelName)
	require.NoError(t, err)
	require.NotNil(t, rec)
	// 比较 JSON 内容
	var promptValue string
	err = json.Unmarshal(rec.Request, &promptValue)
	require.NoError(t, err)
	assert.Equal(t, prompt, promptValue)
	assert.Equal(t, modelName, rec.ModelName)
	assert.Equal(t, response, string(rec.Response))
	if rec.TotalTokens != nil {
		assert.Equal(t, totalTokens, *rec.TotalTokens)
	}
	if rec.PromptTokens != nil {
		assert.Equal(t, promptTokens, *rec.PromptTokens)
	}
	if rec.CompletionTokens != nil {
		assert.Equal(t, completionTokens, *rec.CompletionTokens)
	}

	key := "llm:" + utils.MakeHash(string(promptJSON))
	rdb := newRawRedis()
	defer rdb.Close()
	val, err := rdb.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.NotEmpty(t, val)
}

func TestStorage_GetLLM_ReadThrough(t *testing.T) {
	s := setupTestStorage(t)
	defer s.Close()

	ctx := context.Background()
	prompt := "readthrough test"
	modelName := "gpt-4"
	temperature := float32(0.5)
	maxTokens := 128
	response := `{"result": "ok"}`
	totalTokens := 1
	promptTokens := 1
	completionTokens := 0

	// 将 prompt 转换为 JSON 格式
	promptJSON, err := json.Marshal(prompt)
	require.NoError(t, err)
	responseJSON := json.RawMessage(response)

	// Ensure DB has the record
	llmRecord := &db.LLMRecord{
		Request:          json.RawMessage(promptJSON),
		ModelName:        modelName,
		Temperature:      &temperature,
		MaxTokens:        &maxTokens,
		Response:         responseJSON,
		TotalTokens:      &totalTokens,
		PromptTokens:     &promptTokens,
		CompletionTokens: &completionTokens,
	}
	require.NoError(t, s.UpsertLLM(ctx, llmRecord))

	// Remove from Redis to test backfill
	key := "llm:" + utils.MakeHash(string(promptJSON))
	rdb := newRawRedis()
	defer rdb.Close()
	_ = rdb.Del(ctx, key).Err()

	// First Get from DB
	rec, err := s.GetLLM(ctx, string(promptJSON), modelName)
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, response, string(rec.Response))

	// Ensure backfilled to Redis
	res, err := rdb.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), res)
}

func TestStorage_LLMFlow_WithNilParams(t *testing.T) {
	s := setupTestStorage(t)
	defer s.Close()

	ctx := context.Background()
	prompt := "Test with nil params"
	modelName := "gpt-4"
	response := `{"message":"Response without params"}`

	// 将 prompt 转换为 JSON 格式
	promptJSON, err := json.Marshal(prompt)
	require.NoError(t, err)
	responseJSON := json.RawMessage(response)

	// Upsert with nil optional parameters
	llmRecord := &db.LLMRecord{
		Request:   promptJSON,
		ModelName: modelName,
		Response:  responseJSON,
	}
	require.NoError(t, s.UpsertLLM(ctx, llmRecord))

	// Read back with same nil parameters
	rec, err := s.GetLLM(ctx, string(promptJSON), modelName)
	require.NoError(t, err)
	require.NotNil(t, rec)
	var promptValue string
	err = json.Unmarshal(rec.Request, &promptValue)
	require.NoError(t, err)
	assert.Equal(t, prompt, promptValue)
	assert.Equal(t, modelName, rec.ModelName)
	assert.Equal(t, response, string(rec.Response))
	assert.Nil(t, rec.Temperature)
	assert.Nil(t, rec.MaxTokens)
	assert.Nil(t, rec.TotalTokens)
	assert.Nil(t, rec.PromptTokens)
	assert.Nil(t, rec.CompletionTokens)
}

func TestStorage_LLMFlow_MixedParams(t *testing.T) {
	s := setupTestStorage(t)
	defer s.Close()

	ctx := context.Background()
	prompt := "Test with mixed params"
	modelName := "gpt-4"
	temperature := float32(0.0) // 测试零值和 nil 的区别
	response := `{"result": "Response with only temperature"}`

	// 将 prompt 转换为 JSON 格式
	promptJSON, err := json.Marshal(prompt)
	require.NoError(t, err)
	responseJSON := json.RawMessage(response)

	// Upsert with only temperature set
	llmRecord := &db.LLMRecord{
		Request:     json.RawMessage(promptJSON),
		ModelName:   modelName,
		Temperature: &temperature,
		Response:    responseJSON,
	}
	require.NoError(t, s.UpsertLLM(ctx, llmRecord))

	// Read back
	rec, err := s.GetLLM(ctx, string(promptJSON), modelName)
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.NotNil(t, rec.Temperature)
	assert.Equal(t, float32(0.0), *rec.Temperature) // 确认 0.0 是有意义的值
	assert.Nil(t, rec.MaxTokens)
	assert.Nil(t, rec.TotalTokens)
	assert.Nil(t, rec.PromptTokens)
	assert.Nil(t, rec.CompletionTokens)
}

func TestStorage_Close(t *testing.T) {
	s := setupTestStorage(t)
	// Ensure Close is safe to call multiple times
	s.Close()
	s.Close()
}
