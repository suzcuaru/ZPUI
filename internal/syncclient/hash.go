package syncclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// HashJSON — стабильный хеш payload для секций «по изменению»:
// json.Marshal карт карт сортирует ключи, сериализация детерминирована.
// Поля collected_at вычитываются вызывающим (иначе хеш менялся бы каждый тик).
func HashJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
