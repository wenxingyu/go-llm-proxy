package db

import (
	"encoding/json"
	"time"
)

// EmbeddingRecord represents an embedding cache record
type EmbeddingRecord struct {
	ID        int        `json:"id"`
	InputHash string     `json:"input_hash"`
	InputText string     `json:"input_text"`
	ModelName string     `json:"model_name"`
	Embedding []float64  `json:"embedding"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpireAt  *time.Time `json:"expire_at,omitempty"`
}

// LLMRecord represents an LLM cache record
type LLMRecord struct {
	ID               int             `json:"id"`
	RequestID        string          `json:"request_id"` // 请求 ID
	RequestHash      string          `json:"request_hash"`
	Prompt           json.RawMessage `json:"prompt"` // JSONB 格式的 prompt
	ModelName        string          `json:"model_name"`
	Temperature      *float32        `json:"temperature,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Response         json.RawMessage `json:"response"`                    // JSONB 格式的 response
	TotalTokens      *int            `json:"total_tokens,omitempty"`      // 总 token 数
	PromptTokens     *int            `json:"prompt_tokens,omitempty"`     // prompt token 数
	CompletionTokens *int            `json:"completion_tokens,omitempty"` // completion token 数
	StartTime        *time.Time      `json:"start_time,omitempty"`        // 请求开始时间
	EndTime          *time.Time      `json:"end_time,omitempty"`          // 请求结束时间
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	ExpireAt         *time.Time      `json:"expire_at,omitempty"`
}

const ddl = `
CREATE TABLE IF NOT EXISTS embedding_cache (
    id SERIAL PRIMARY KEY,
    input_hash CHAR(64) NOT NULL,        -- 输入文本的哈希值（如 SHA256）
    input_text TEXT NOT NULL,            -- 原始输入，方便调试
    model_name VARCHAR(128) NOT NULL,    -- 模型名称
    embedding FLOAT8[] NOT NULL,         -- 用数组存向量
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    expire_at TIMESTAMP,                 -- 可选，过期时间
    UNIQUE(input_hash, model_name)       -- 保证唯一
);
CREATE TABLE IF NOT EXISTS llm_cache (
    id SERIAL PRIMARY KEY,
    request_id VARCHAR(255),             -- 请求 ID
    request_hash CHAR(64) NOT NULL,      -- 对 prompt+参数 做 hash
    prompt JSONB NOT NULL,               -- 原始 prompt (JSON格式)
    model_name VARCHAR(128) NOT NULL,    -- 模型名称
    temperature NUMERIC(3,2),            -- 可选参数
    max_tokens INT,                      -- 可选参数
    response JSONB NOT NULL,              -- 模型返回的文本 (JSON格式)
    total_tokens INT,                    -- 总 token 数
    prompt_tokens INT,                   -- prompt token 数
    completion_tokens INT,               -- completion token 数
    start_time TIMESTAMP(3),             -- 请求开始时间
    end_time TIMESTAMP(3),               -- 请求结束时间
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    expire_at TIMESTAMP,                 -- 可选，缓存过期
    UNIQUE(request_hash, model_name)     -- 保证唯一
);
`
