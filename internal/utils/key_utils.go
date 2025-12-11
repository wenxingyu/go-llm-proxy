package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func MakeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// MakeEmbeddingCacheKey builds the deterministic hash for embedding cache rows.
// It joins the normalized input payload with model and optional dimensions so caches stay unique per model/dimension.
func MakeEmbeddingCacheKey(inputText, modelName string, dimensions *int) string {
	key := inputText + "|" + modelName
	if dimensions != nil {
		key = key + fmt.Sprintf("|%d", *dimensions)
	}
	return MakeHash(key)
}
