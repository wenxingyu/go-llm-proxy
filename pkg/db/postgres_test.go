package db

import (
	"context"
	"fmt"
	"os"
	"testing"

	"go-llm-server/internal/config"
	"go-llm-server/internal/utils"
)

//	image: postgres:17
//	env:
//	  POSTGRES_USER: test
//	  POSTGRES_PASSWORD: test
//	  POSTGRES_DB: testdb
//
// setupTestDB creates a test database connection
// Note: This requires a running PostgreSQL instance for integration tests
func setupTestDB(t *testing.T) *Postgres {
	// Use test database configuration
	// In a real test environment, you might want to use testcontainers
	// or a dedicated test database
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
		t.Skipf("Skipping test due to database connection error: %v", err)
	}

	return pg
}

// equalFloatSlices compares two float64 slices for equality
func equalFloatSlices(a, b []float64) bool {
	// Handle nil cases
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// generateLongText generates a long UTF8 string for testing
func generateLongText(length int) string {
	// Use a repeating pattern of valid UTF8 characters
	pattern := "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = pattern[i%len(pattern)]
	}
	return string(result)
}

func newTestEmbeddingRecord(inputText, modelName string, embedding []float64) *EmbeddingRecord {
	return &EmbeddingRecord{
		InputText: inputText,
		ModelName: modelName,
		Embedding: embedding,
	}
}

// cleanupTestDB cleans up test data
func cleanupTestDB(t *testing.T, pg *Postgres) {
	ctx := context.Background()

	// Clean up test data
	_, err := pg.Pool.Exec(ctx, "DELETE FROM embedding_cache WHERE input_text LIKE 'test_%' OR input_text LIKE 'benchmark_%'")
	if err != nil {
		t.Logf("Warning: failed to cleanup test data: %v", err)
	}
}

func TestUpsertEmbedding(t *testing.T) {
	pg := setupTestDB(t)
	defer pg.Close()
	defer cleanupTestDB(t, pg)

	ctx := context.Background()

	tests := []struct {
		name        string
		inputText   string
		modelName   string
		embedding   []float64
		expectError bool
	}{
		{
			name:        "valid embedding insertion",
			inputText:   "test_hello_world",
			modelName:   "text-embedding-ada-002",
			embedding:   []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			expectError: false,
		},
		{
			name:        "empty embedding",
			inputText:   "test_empty_embedding",
			modelName:   "text-embedding-ada-002",
			embedding:   []float64{},
			expectError: false,
		},
		{
			name:        "large embedding",
			inputText:   "test_large_embedding",
			modelName:   "text-embedding-ada-002",
			embedding:   make([]float64, 1536), // Common embedding size
			expectError: false,
		},
		{
			name:        "different model",
			inputText:   "test_hello_world",
			modelName:   "text-embedding-3-small",
			embedding:   []float64{0.6, 0.7, 0.8, 0.9, 1.0},
			expectError: false,
		},
		{
			name:        "special characters in input",
			inputText:   "test_特殊字符!@#$%^&*()",
			modelName:   "text-embedding-ada-002",
			embedding:   []float64{0.1, 0.2, 0.3},
			expectError: false,
		},
		{
			name:        "very long input text",
			inputText:   "test_" + generateLongText(10000),
			modelName:   "text-embedding-ada-002",
			embedding:   []float64{0.1, 0.2, 0.3},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(tt.inputText, tt.modelName, tt.embedding))

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestUpsertEmbeddingUpdate(t *testing.T) {
	pg := setupTestDB(t)
	defer pg.Close()
	defer cleanupTestDB(t, pg)

	ctx := context.Background()
	inputText := "test_update_embedding"
	modelName := "text-embedding-ada-002"
	originalEmbedding := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	updatedEmbedding := []float64{0.6, 0.7, 0.8, 0.9, 1.0}

	// First insertion
	err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(inputText, modelName, originalEmbedding))
	if err != nil {
		t.Fatalf("First insertion should succeed: %v", err)
	}

	// Verify first insertion
	record, err := pg.GetEmbedding(ctx, inputText, modelName)
	if err != nil {
		t.Fatalf("Should be able to retrieve first embedding: %v", err)
	}
	if !equalFloatSlices(originalEmbedding, record.Embedding) {
		t.Errorf("First embedding should match: got %v, expected %v", record.Embedding, originalEmbedding)
	}

	// Update with new embedding
	err = pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(inputText, modelName, updatedEmbedding))
	if err != nil {
		t.Fatalf("Update should succeed: %v", err)
	}

	// Verify update
	record, err = pg.GetEmbedding(ctx, inputText, modelName)
	if err != nil {
		t.Fatalf("Should be able to retrieve updated embedding: %v", err)
	}
	if !equalFloatSlices(updatedEmbedding, record.Embedding) {
		t.Errorf("Updated embedding should match: got %v, expected %v", record.Embedding, updatedEmbedding)
	}
	if !record.UpdatedAt.After(record.CreatedAt) {
		t.Errorf("UpdatedAt should be after CreatedAt")
	}
}

func TestGetEmbedding(t *testing.T) {
	pg := setupTestDB(t)
	defer pg.Close()
	defer cleanupTestDB(t, pg)

	ctx := context.Background()

	// Setup test data
	testCases := []struct {
		inputText string
		modelName string
		embedding []float64
	}{
		{
			inputText: "test_hello_world",
			modelName: "text-embedding-ada-002",
			embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		},
		{
			inputText: "test_different_model",
			modelName: "text-embedding-3-small",
			embedding: []float64{0.6, 0.7, 0.8, 0.9, 1.0},
		},
		{
			inputText: "test_empty_embedding",
			modelName: "text-embedding-ada-002",
			embedding: []float64{},
		},
	}

	// Insert test data
	for _, tc := range testCases {
		err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(tc.inputText, tc.modelName, tc.embedding))
		if err != nil {
			t.Fatalf("Setup should succeed for %s: %v", tc.inputText, err)
		}
	}

	tests := []struct {
		name        string
		inputText   string
		modelName   string
		expectFound bool
		expectError bool
	}{
		{
			name:        "existing embedding",
			inputText:   "test_hello_world",
			modelName:   "text-embedding-ada-002",
			expectFound: true,
			expectError: false,
		},
		{
			name:        "different model for same text",
			inputText:   "test_hello_world",
			modelName:   "text-embedding-3-small",
			expectFound: false,
			expectError: true, // Should return error for not found
		},
		{
			name:        "non-existent text",
			inputText:   "test_non_existent",
			modelName:   "text-embedding-ada-002",
			expectFound: false,
			expectError: true,
		},
		{
			name:        "non-existent model",
			inputText:   "test_hello_world",
			modelName:   "non-existent-model",
			expectFound: false,
			expectError: true,
		},
		{
			name:        "empty input text",
			inputText:   "",
			modelName:   "text-embedding-ada-002",
			expectFound: false,
			expectError: true,
		},
		{
			name:        "empty model name",
			inputText:   "test_hello_world",
			modelName:   "",
			expectFound: false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := pg.GetEmbedding(ctx, tt.inputText, tt.modelName)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if record != nil {
					t.Errorf("Record should be nil when error occurs")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if record == nil {
					t.Errorf("Record should not be nil")
				}

				if tt.expectFound && record != nil {
					if record.InputText != tt.inputText {
						t.Errorf("InputText should match: got %s, expected %s", record.InputText, tt.inputText)
					}
					if record.ModelName != tt.modelName {
						t.Errorf("ModelName should match: got %s, expected %s", record.ModelName, tt.modelName)
					}
					if record.InputHash == "" {
						t.Errorf("InputHash should not be empty")
					}
					if record.ID == 0 {
						t.Errorf("ID should not be zero")
					}
					if record.CreatedAt.IsZero() {
						t.Errorf("CreatedAt should not be zero")
					}
					if record.UpdatedAt.IsZero() {
						t.Errorf("UpdatedAt should not be zero")
					}
				}
			}
		})
	}
}

func TestGetEmbeddingDataIntegrity(t *testing.T) {
	pg := setupTestDB(t)
	defer pg.Close()
	defer cleanupTestDB(t, pg)

	ctx := context.Background()

	inputText := "test_data_integrity"
	modelName := "text-embedding-ada-002"
	embedding := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}

	// Insert embedding
	err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(inputText, modelName, embedding))
	if err != nil {
		t.Fatalf("Insert should succeed: %v", err)
	}

	// Retrieve and verify data integrity
	record, err := pg.GetEmbedding(ctx, inputText, modelName)
	if err != nil {
		t.Fatalf("Retrieve should succeed: %v", err)
	}
	if record == nil {
		t.Fatalf("Record should not be nil")
	}

	// Verify all fields
	if record.InputText != inputText {
		t.Errorf("InputText should match: got %s, expected %s", record.InputText, inputText)
	}
	if record.ModelName != modelName {
		t.Errorf("ModelName should match: got %s, expected %s", record.ModelName, modelName)
	}
	if !equalFloatSlices(embedding, record.Embedding) {
		t.Errorf("Embedding should match exactly: got %v, expected %v", record.Embedding, embedding)
	}

	// Verify hash calculation
	expectedHash := utils.MakeEmbeddingCacheKey(inputText, modelName)
	if record.InputHash != expectedHash {
		t.Errorf("InputHash should match calculated hash: got %s, expected %s", record.InputHash, expectedHash)
	}

	// Verify timestamps
	if record.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should not be zero")
	}
	if record.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt should not be zero")
	}
	if !record.UpdatedAt.Equal(record.CreatedAt) && !record.UpdatedAt.After(record.CreatedAt) {
		t.Errorf("UpdatedAt should be equal to or after CreatedAt")
	}

	// Verify ID is set
	if record.ID <= 0 {
		t.Errorf("ID should be greater than 0, got %d", record.ID)
	}
}

func TestEmbeddingHashConsistency(t *testing.T) {
	pg := setupTestDB(t)
	defer pg.Close()
	defer cleanupTestDB(t, pg)

	ctx := context.Background()

	// Test that the same input always produces the same hash
	inputText := "test_hash_consistency"
	modelName := "text-embedding-ada-002"
	embedding := []float64{0.1, 0.2, 0.3}

	// Insert multiple times with same input
	for i := 0; i < 3; i++ {
		err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(inputText, modelName, embedding))
		if err != nil {
			t.Fatalf("Insert %d should succeed: %v", i+1, err)
		}
	}

	// Retrieve and verify hash consistency
	record, err := pg.GetEmbedding(ctx, inputText, modelName)
	if err != nil {
		t.Fatalf("Retrieve should succeed: %v", err)
	}

	expectedHash := utils.MakeEmbeddingCacheKey(inputText, modelName)
	if record.InputHash != expectedHash {
		t.Errorf("Hash should be consistent: got %s, expected %s", record.InputHash, expectedHash)
	}

	// Verify only one record exists (due to unique constraint)
	count, err := pg.CountEmbeddings(ctx, modelName)
	if err != nil {
		t.Fatalf("Count should succeed: %v", err)
	}
	if count != 1 {
		t.Errorf("Should have exactly one record, got %d", count)
	}
}

func TestEmbeddingConcurrentAccess(t *testing.T) {
	pg := setupTestDB(t)
	defer pg.Close()
	defer cleanupTestDB(t, pg)

	ctx := context.Background()
	inputText := "test_concurrent"
	modelName := "text-embedding-ada-002"
	embedding := []float64{0.1, 0.2, 0.3}

	// Test concurrent upserts
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(inputText, modelName, embedding))
			done <- err
		}()
	}

	// Wait for all goroutines to complete and check for errors
	for i := 0; i < 10; i++ {
		err := <-done
		if err != nil {
			t.Errorf("Concurrent upsert should succeed: %v", err)
		}
	}

	// Verify final state
	record, err := pg.GetEmbedding(ctx, inputText, modelName)
	if err != nil {
		t.Fatalf("Retrieve should succeed: %v", err)
	}
	if !equalFloatSlices(embedding, record.Embedding) {
		t.Errorf("Final embedding should match: got %v, expected %v", record.Embedding, embedding)
	}
}

func TestEmbeddingEdgeCases(t *testing.T) {
	pg := setupTestDB(t)
	defer pg.Close()
	defer cleanupTestDB(t, pg)

	ctx := context.Background()

	tests := []struct {
		name        string
		inputText   string
		modelName   string
		embedding   []float64
		expectError bool
	}{
		{
			name:        "very long model name",
			inputText:   "test_long_model",
			modelName:   generateLongText(120), // Long model name within VARCHAR(128) limit
			embedding:   []float64{0.1, 0.2, 0.3},
			expectError: false,
		},
		{
			name:        "unicode input text",
			inputText:   "test_测试_🚀_emoji_中文",
			modelName:   "text-embedding-ada-002",
			embedding:   []float64{0.1, 0.2, 0.3},
			expectError: false,
		},
		{
			name:        "negative values in embedding",
			inputText:   "test_negative_values",
			modelName:   "text-embedding-ada-002",
			embedding:   []float64{-1.0, -0.5, 0.0, 0.5, 1.0},
			expectError: false,
		},
		{
			name:        "very large values in embedding",
			inputText:   "test_large_values",
			modelName:   "text-embedding-ada-002",
			embedding:   []float64{1e10, -1e10, 0.0},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test upsert
			err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(tt.inputText, tt.modelName, tt.embedding))
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.name, err)
				} else {
					// Test retrieval if upsert succeeded
					record, err := pg.GetEmbedding(ctx, tt.inputText, tt.modelName)
					if err != nil {
						t.Errorf("Retrieve should succeed for %s: %v", tt.name, err)
					} else if !equalFloatSlices(tt.embedding, record.Embedding) {
						t.Errorf("Embedding should match for %s: got %v, expected %v", tt.name, record.Embedding, tt.embedding)
					}
				}
			}
		})
	}
}

// BenchmarkUpsertEmbedding benchmarks the UpsertEmbedding method
func BenchmarkUpsertEmbedding(b *testing.B) {
	pg := setupTestDB(&testing.T{})
	if pg == nil {
		b.Skip("Skipping benchmark due to database connection error")
	}
	defer pg.Close()

	ctx := context.Background()
	embedding := make([]float64, 1536) // Common embedding size
	for i := range embedding {
		embedding[i] = float64(i) / 1536.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inputText := fmt.Sprintf("benchmark_%d", i)
		err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(inputText, "text-embedding-ada-002", embedding))
		if err != nil {
			b.Fatalf("UpsertEmbedding failed: %v", err)
		}
	}

	// Clean up benchmark data
	_, err := pg.Pool.Exec(ctx, "DELETE FROM embedding_cache WHERE input_text LIKE 'benchmark_%'")
	if err != nil {
		b.Logf("Warning: failed to cleanup benchmark data: %v", err)
	}
}

// BenchmarkGetEmbedding benchmarks the GetEmbedding method
func BenchmarkGetEmbedding(b *testing.B) {
	pg := setupTestDB(&testing.T{})
	if pg == nil {
		b.Skip("Skipping benchmark due to database connection error")
	}
	defer pg.Close()

	ctx := context.Background()

	// Setup test data
	embedding := make([]float64, 1536)
	for i := range embedding {
		embedding[i] = float64(i) / 1536.0
	}

	inputText := "benchmark_get_test"
	modelName := "text-embedding-ada-002"

	err := pg.UpsertEmbedding(ctx, newTestEmbeddingRecord(inputText, modelName, embedding))
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pg.GetEmbedding(ctx, inputText, modelName)
		if err != nil {
			b.Fatalf("GetEmbedding failed: %v", err)
		}
	}

	// Clean up benchmark data
	_, err = pg.Pool.Exec(ctx, "DELETE FROM embedding_cache WHERE input_text = 'benchmark_get_test'")
	if err != nil {
		b.Logf("Warning: failed to cleanup benchmark data: %v", err)
	}
}
