package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

type EmbeddingCache struct {
	InputHash string
	InputText string
	ModelName string
	Embedding []float64
}

type LlmCache struct {
	RequestHash string
	Prompt      string
	ModelName   string
	Temperature float32
	MaxTokens   int
	Response    string
	TokensUsed  int
}

func makeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ---------- client 包装 ----------
func redisGet[T any](ctx context.Context, rdb *redis.Client, key string, dest *T) (bool, error) {
	val, err := rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(val), dest)
}

func redisSet(ctx context.Context, rdb *redis.Client, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, key, data, ttl).Err()
}

// ---------- Embedding 缓存 ----------
func getEmbedding(ctx context.Context, db *sql.DB, rdb *redis.Client, inputText, modelName string) (*EmbeddingCache, error) {
	hash := makeHash(inputText + modelName)
	key := "embedding:" + hash

	// 1. 查 client
	var ec EmbeddingCache
	found, err := redisGet(ctx, rdb, key, &ec)
	if err != nil {
		return nil, err
	}
	if found {
		return &ec, nil
	}

	// 2. 查 PG
	row := db.QueryRowContext(ctx, `
		SELECT input_hash, input_text, model_name, embedding
		FROM embedding_cache
		WHERE input_hash=$1 AND model_name=$2
	`, hash, modelName)

	err = row.Scan(&ec.InputHash, &ec.InputText, &ec.ModelName, &ec.Embedding)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// 3. 写回 client
	_ = redisSet(ctx, rdb, key, ec, time.Hour)

	return &ec, nil
}

func upsertEmbedding(ctx context.Context, db *sql.DB, rdb *redis.Client, inputText, modelName string, embedding []float64) error {
	hash := makeHash(inputText + modelName)
	key := "embedding:" + hash

	_, err := db.ExecContext(ctx, `
		INSERT INTO embedding_cache (input_hash, input_text, model_name, embedding)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (input_hash, model_name)
		DO UPDATE SET embedding = EXCLUDED.embedding, updated_at = NOW()
	`, hash, inputText, modelName, embedding)
	if err != nil {
		return err
	}

	// 写 client
	ec := EmbeddingCache{InputHash: hash, InputText: inputText, ModelName: modelName, Embedding: embedding}
	return redisSet(ctx, rdb, key, ec, time.Hour)
}

// ---------- LLM 缓存 ----------
func getLLM(ctx context.Context, db *sql.DB, rdb *redis.Client, prompt, modelName string, temperature float32, maxTokens int) (*LlmCache, error) {
	hash := makeHash(fmt.Sprintf("%s|%s|%f|%d", prompt, modelName, temperature, maxTokens))
	key := "llm:" + hash

	// 1. 查 client
	var lc LlmCache
	found, err := redisGet(ctx, rdb, key, &lc)
	if err != nil {
		return nil, err
	}
	if found {
		return &lc, nil
	}

	// 2. 查 PG
	row := db.QueryRowContext(ctx, `
		SELECT request_hash, prompt, model_name, temperature, max_tokens, response, tokens_used
		FROM llm_cache
		WHERE request_hash=$1 AND model_name=$2
	`, hash, modelName)

	err = row.Scan(&lc.RequestHash, &lc.Prompt, &lc.ModelName, &lc.Temperature, &lc.MaxTokens, &lc.Response, &lc.TokensUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// 3. 写回 client
	_ = redisSet(ctx, rdb, key, lc, time.Hour)

	return &lc, nil
}

func upsertLLM(ctx context.Context, db *sql.DB, rdb *redis.Client, prompt, modelName string, temperature float32, maxTokens int, response string, tokensUsed int) error {
	hash := makeHash(fmt.Sprintf("%s|%s|%f|%d", prompt, modelName, temperature, maxTokens))
	key := "llm:" + hash

	_, err := db.ExecContext(ctx, `
		INSERT INTO llm_cache (request_hash, prompt, model_name, temperature, max_tokens, response, tokens_used)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (request_hash, model_name)
		DO UPDATE SET response = EXCLUDED.response, tokens_used = EXCLUDED.tokens_used, updated_at = NOW()
	`, hash, prompt, modelName, temperature, maxTokens, response, tokensUsed)
	if err != nil {
		return err
	}

	// 写 client
	lc := LlmCache{RequestHash: hash, Prompt: prompt, ModelName: modelName, Temperature: temperature, MaxTokens: maxTokens, Response: response, TokensUsed: tokensUsed}
	return redisSet(ctx, rdb, key, lc, time.Hour)
}

func main() {
	ctx := context.Background()

	// 连接 PG
	pgConnStr := "postgres://user:password@localhost:5432/mydb?sslmode=disable"
	db, err := sql.Open("pgx", pgConnStr)
	if err != nil {
		log.Fatal("failed to connect PG:", err)
	}
	defer db.Close()

	// 连接 client
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	defer rdb.Close()

	// 示例：写入 & 查询 Embedding
	_ = upsertEmbedding(ctx, db, rdb, "hello world", "text-embedding-model", []float64{0.1, 0.2, 0.3})
	ec, _ := getEmbedding(ctx, db, rdb, "hello world", "text-embedding-model")
	fmt.Println("Embedding cache:", ec)

	// 示例：写入 & 查询 LLM
	_ = upsertLLM(ctx, db, rdb, "What is Go?", "gpt-4", 0.7, 512, "Go is a programming language ...", 42)
	lc, _ := getLLM(ctx, db, rdb, "What is Go?", "gpt-4", 0.7, 512)
	fmt.Println("LLM cache:", lc)
}
