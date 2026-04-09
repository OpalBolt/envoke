package bitwarden

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
