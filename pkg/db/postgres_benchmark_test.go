package db

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"go-llm-server/internal/config"
)

// BenchmarkUpsertLLM_WithPreparedStatement benchmarks upsert operations
// to demonstrate the performance benefit of prepared statements
func BenchmarkUpsertLLM_WithPreparedStatement(b *testing.B) {
	cfg := config.DatabaseConfig{
		Host:            os.Getenv("DB_HOST"),
		Port:            5432,
		User:            os.Getenv("DB_USER"),
		Password:        os.Getenv("DB_PASSWORD"),
		DBName:          os.Getenv("DB_NAME"),
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 300,
	}

	pg, err := NewPostgres(cfg)
	if err != nil {
		b.Skipf("skipping benchmark: %v", err)
	}
	defer pg.Close()

	ctx := context.Background()
	temperature := float32(0.7)
	maxTokens := 100
	totalTokens := 50
	promptTokens := 20
	completionTokens := 30

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prompt := fmt.Sprintf("benchmark prompt %d", i)
		promptJSON, _ := json.Marshal(prompt)
		responseJSON, _ := json.Marshal(map[string]string{"response": "response"})
		rec := &LLMRecord{
			Request:          json.RawMessage(promptJSON),
			ModelName:        "gpt-4",
			Temperature:      &temperature,
			MaxTokens:        &maxTokens,
			Response:         json.RawMessage(responseJSON),
			TotalTokens:      &totalTokens,
			PromptTokens:     &promptTokens,
			CompletionTokens: &completionTokens,
		}
		_ = pg.UpsertLLM(ctx, rec)
	}
}

// BenchmarkGetLLM_WithPreparedStatement benchmarks get operations
func BenchmarkGetLLM_WithPreparedStatement(b *testing.B) {
	cfg := config.DatabaseConfig{
		Host:            "192.168.70.128",
		Port:            5432,
		User:            "postgres",
		Password:        "postgres_password",
		DBName:          "postgres_test",
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 300,
	}

	pg, err := NewPostgres(cfg)
	if err != nil {
		b.Skipf("skipping benchmark: %v", err)
	}
	defer pg.Close()

	ctx := context.Background()
	temperature := float32(0.7)
	maxTokens := 100
	totalTokens := 50
	promptTokens := 20
	completionTokens := 30

	// Insert test data
	prompt := "benchmark get test"
	promptJSON, _ := json.Marshal(prompt)
	responseJSON, _ := json.Marshal(map[string]string{"response": "response"})
	rec := &LLMRecord{
		Request:          json.RawMessage(promptJSON),
		ModelName:        "gpt-4",
		Temperature:      &temperature,
		MaxTokens:        &maxTokens,
		Response:         json.RawMessage(responseJSON),
		TotalTokens:      &totalTokens,
		PromptTokens:     &promptTokens,
		CompletionTokens: &completionTokens,
	}
	_ = pg.UpsertLLM(ctx, rec)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pg.GetLLM(ctx, prompt)
	}
}

// BenchmarkUpsertEmbedding_WithPreparedStatement benchmarks embedding upserts
func BenchmarkUpsertEmbedding_WithPreparedStatement(b *testing.B) {
	cfg := config.DatabaseConfig{
		Host:            "192.168.70.128",
		Port:            5432,
		User:            "postgres",
		Password:        "postgres_password",
		DBName:          "postgres_test",
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 300,
	}

	pg, err := NewPostgres(cfg)
	if err != nil {
		b.Skipf("skipping benchmark: %v", err)
	}
	defer pg.Close()

	ctx := context.Background()
	embedding := make([]float64, 1536) // Typical OpenAI embedding size
	for i := range embedding {
		embedding[i] = float64(i) / 1536.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inputText := fmt.Sprintf("benchmark embedding %d", i)
		_ = pg.UpsertEmbedding(ctx, inputText, "text-embedding-ada-002", embedding)
	}
}
