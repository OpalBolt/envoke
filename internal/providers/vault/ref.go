package vault

import (
	"fmt"
	"strings"
)

// VaultRef holds the parsed components of a vault:// URI.
type VaultRef struct {
	Path  string
	Field string
}

// ParseVaultRef parses a vault://path#field URI.
// Returns an error if the field fragment is missing.
func ParseVaultRef(uri string) (VaultRef, error) {
	if !strings.HasPrefix(uri, "vault://") {
		return VaultRef{}, fmt.Errorf("not a vault:// URI: %q", uri)
	}
	rest := strings.TrimPrefix(uri, "vault://")
	idx := strings.LastIndex(rest, "#")
	if idx < 0 {
		return VaultRef{}, fmt.Errorf("vault:// URI missing #field fragment: %q", uri)
	}
	path := rest[:idx]
	field := rest[idx+1:]
	if path == "" {
		return VaultRef{}, fmt.Errorf("vault:// URI has empty path: %q", uri)
	}
	if field == "" {
		return VaultRef{}, fmt.Errorf("vault:// URI has empty field: %q", uri)
	}
	return VaultRef{Path: path, Field: field}, nil
}
