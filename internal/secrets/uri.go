package secrets

import (
	"fmt"
	"strings"
)

// BWRef holds the parsed components of a bw:// URI.
type BWRef struct {
	Folder       string
	Item         string
	FieldSpec    string // "password" (default), "username", "note", "totp", "field:<name>"
	IsCollection bool   // true if bw://collection:<name>/item
}

// VaultRef holds the parsed components of a vault:// URI.
type VaultRef struct {
	Path  string
	Field string
}

// ParseBWRef parses a bw://folder/item-name[/field-spec] URI.
// Returns an error if folder or item is empty, or format is invalid.
func ParseBWRef(uri string) (BWRef, error) {
	if !strings.HasPrefix(uri, "bw://") {
		return BWRef{}, fmt.Errorf("not a bw:// URI: %q", uri)
	}
	rest := strings.TrimPrefix(uri, "bw://")
	// Check for collection: prefix
	isCollection := false
	if strings.HasPrefix(rest, "collection:") {
		isCollection = true
		rest = strings.TrimPrefix(rest, "collection:")
	}

	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return BWRef{}, fmt.Errorf("bw:// URI must have at least folder and item: %q", uri)
	}
	folder := parts[0]
	item := parts[1]
	if folder == "" {
		return BWRef{}, fmt.Errorf("bw:// URI has empty folder: %q", uri)
	}
	if item == "" {
		return BWRef{}, fmt.Errorf("bw:// URI has empty item: %q", uri)
	}
	fieldSpec := "password" // default
	if len(parts) == 3 && parts[2] != "" {
		fieldSpec = parts[2]
	}
	return BWRef{Folder: folder, Item: item, FieldSpec: fieldSpec, IsCollection: isCollection}, nil
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
	if field == "" {
		return VaultRef{}, fmt.Errorf("vault:// URI has empty field: %q", uri)
	}
	return VaultRef{Path: path, Field: field}, nil
}

// IsSecretRef returns true if s starts with "bw://" or "vault://".
func IsSecretRef(s string) bool {
	return strings.HasPrefix(s, "bw://") || strings.HasPrefix(s, "vault://")
}
