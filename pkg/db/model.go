package db

import "time"

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
	ID          int        `json:"id"`
	RequestHash string     `json:"request_hash"`
	Prompt      string     `json:"prompt"`
	ModelName   string     `json:"model_name"`
	Temperature *float32   `json:"temperature,omitempty"`
	MaxTokens   *int       `json:"max_tokens,omitempty"`
	Response    string     `json:"response"`
	TokensUsed  *int       `json:"tokens_used,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ExpireAt    *time.Time `json:"expire_at,omitempty"`
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
    request_hash CHAR(64) NOT NULL,      -- 对 prompt+参数 做 hash
    prompt TEXT NOT NULL,                -- 原始 prompt
    model_name VARCHAR(128) NOT NULL,    -- 模型名称
    temperature NUMERIC(3,2),            -- 可选参数
    max_tokens INT,                      -- 可选参数
    response TEXT NOT NULL,              -- 模型返回的文本
    tokens_used INT,                     -- 可选，统计 token 消耗
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    expire_at TIMESTAMP,                 -- 可选，缓存过期
    UNIQUE(request_hash, model_name)     -- 保证唯一
);
`
