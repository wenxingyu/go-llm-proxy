package db

import (
	"encoding/json"
	"time"
)

// EmbeddingRecord represents an embedding cache record
type EmbeddingRecord struct {
	ID         int        `json:"id"`
	InputHash  string     `json:"input_hash"`
	InputText  string     `json:"input_text"`
	ModelName  string     `json:"model_name"`
	Provider   string     `json:"provider"`
	RequestID  string     `json:"request_id"`
	TokenCount *int       `json:"token_count,omitempty"`
	Embedding  []float64  `json:"embedding"`
	StartTime  *time.Time `json:"start_time,omitempty"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	DurationMs *int       `json:"duration_ms,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ExpireAt   *int64     `json:"expire_at,omitempty"` // Unix 时间戳（毫秒），-1 表示永不过期
}

// LLMRecord represents an LLM cache record
type LLMRecord struct {
	ID               int             `json:"id"`
	RequestID        string          `json:"request_id"` // 请求 ID
	RequestHash      string          `json:"request_hash"`
	Request          json.RawMessage `json:"request"` // JSONB 格式的 request
	ModelName        string          `json:"model_name"`
	Temperature      *float32        `json:"temperature,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Response         json.RawMessage `json:"response"`                    // JSONB 格式的 response
	TotalTokens      *int            `json:"total_tokens,omitempty"`      // 总 token 数
	PromptTokens     *int            `json:"prompt_tokens,omitempty"`     // prompt token 数
	CompletionTokens *int            `json:"completion_tokens,omitempty"` // completion token 数
	StartTime        *time.Time      `json:"start_time,omitempty"`        // 请求开始时间
	EndTime          *time.Time      `json:"end_time,omitempty"`          // 请求结束时间
	DurationMs       *int            `json:"duration_ms,omitempty"`       // 请求耗时（毫秒）
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	ExpireAt         *int64          `json:"expire_at,omitempty"` // Unix 时间戳（毫秒），-1 表示永不过期
}

const ddl = `
CREATE TABLE IF NOT EXISTS embedding_cache (
    id SERIAL PRIMARY KEY,
    request_id VARCHAR(255),
    -- 核心查找字段
    input_hash CHAR(64) NOT NULL,
    model_name VARCHAR(128) NOT NULL,
    provider   VARCHAR(128) NOT NULL,    
    -- 数据内容
    input_text TEXT NOT NULL,
    embedding REAL[] NOT NULL,
    -- 元数据
    token_count INTEGER,
    -- 时间管理
    created_at TIMESTAMPTZ(3) DEFAULT NOW(),
    updated_at TIMESTAMPTZ(3) DEFAULT NOW(),
    start_time TIMESTAMPTZ(3),
    end_time   TIMESTAMPTZ(3),
    -- 生成列：自动计算耗时 (PostgreSQL 12+ 支持)
    duration_ms INT GENERATED ALWAYS AS (
        CAST(EXTRACT(EPOCH FROM (end_time - start_time)) * 1000 AS INT)
    ) STORED,
    expire_at BIGINT DEFAULT -1,
    -- 联合唯一索引
    UNIQUE(input_hash, model_name, provider)
);
CREATE TABLE IF NOT EXISTS llm_cache (
    id SERIAL PRIMARY KEY,
    request_id VARCHAR(255),             -- 请求 ID
    request_hash CHAR(64) NOT NULL,      -- 对 request 做 hash
    request JSONB NOT NULL,              -- 原始 request (JSON格式)
    model_name VARCHAR(128) NOT NULL,    -- 模型名称
    temperature NUMERIC(3,2),            -- 可选参数
    max_tokens INT,                      -- 可选参数
    response JSONB NOT NULL,             -- 模型返回的文本 (JSON格式)
    total_tokens INT,                    -- 总 token 数
    prompt_tokens INT,                   -- prompt token 数
    completion_tokens INT,               -- completion token 数
    start_time TIMESTAMPTZ(3),             -- 请求开始时间
    end_time TIMESTAMPTZ(3),               -- 请求结束时间
    duration_ms INT GENERATED ALWAYS AS (
        CAST(EXTRACT(EPOCH FROM (end_time - start_time)) * 1000 AS INT)
    ) STORED,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    expire_at BIGINT DEFAULT -1,         -- Unix 时间戳（毫秒），-1 表示永不过期
    UNIQUE(request_hash, model_name)     -- 保证唯一
);
`
