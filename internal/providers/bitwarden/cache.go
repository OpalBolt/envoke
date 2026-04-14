package bitwarden

import (
	"crypto/sha256"
	"encoding/hex"

	appcache "github.com/opalbolt/envoke/internal/cache"
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

// zeroString clears the caller-visible string reference.
// In Go, strings are immutable, so this does not overwrite the original
// backing memory; it only drops this reference to the secret value.
func zeroString(s *string) {
	*s = ""
}
