package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func MakeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
