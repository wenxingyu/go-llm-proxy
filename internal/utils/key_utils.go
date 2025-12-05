package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func MakeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// MakeEmbeddingCacheKey builds the deterministic hash for embedding cache rows.
// It joins the normalized input payload with model so caches stay unique per model.
func MakeEmbeddingCacheKey(inputText, modelName string) string {
	return MakeHash(inputText + "|" + modelName)
}
