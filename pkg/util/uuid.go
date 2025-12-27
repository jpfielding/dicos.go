package util

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
)

// Md5ThenHex is a quick hasher
func Md5ThenHex(value []byte) string {
	hasher := md5.New()
	hasher.Write(value)
	return hex.EncodeToString(hasher.Sum(nil))
}

func HashUUID(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	hasher := md5.New()
	hasher.Write([]byte(raw))
	hash := hasher.Sum(nil)
	uuid, err := uuid.FromBytes(hash[:16])
	if err != nil {
		return ""
	}
	return uuid.String()
}
