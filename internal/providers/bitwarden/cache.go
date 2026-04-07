package bitwarden

import (
	"crypto/sha256"
	"encoding/hex"

	appcache "github.com/eficode/secure-handling-of-secrets/internal/cache"
)

// NewCache creates the shared application cache used to store resolved BW item JSON.
func NewCache() *appcache.Cache {
	return appcache.New()
}

// bwCacheKey returns an opaque cache key for a Bitwarden item lookup.
// Combines uid, account tag, and folder/collection to uniquely identify a cached result.
func bwCacheKey(uid, acctTag, folder string) string {
	h := sha256.Sum256([]byte(uid + ":" + acctTag + ":" + folder))
	return hex.EncodeToString(h[:16])
}

// zeroBytes overwrites a byte slice with zeros.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// zeroString overwrites a string's backing memory with zeros (best-effort in Go).
func zeroString(s *string) {
	b := []byte(*s)
	zeroBytes(b)
	*s = string(make([]byte, len(*s)))
}
