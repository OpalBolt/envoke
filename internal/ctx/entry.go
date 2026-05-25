package ctx

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ContextEntry represents a single CTX_<GROUP>=<uri>#<ENVVAR> directive.
type ContextEntry struct {
	Group       string `json:"group"`
	EnvVar      string `json:"env_var"`
	SourceURI   string `json:"source_uri"`
	TmpfilePath string `json:"tmpfile_path,omitempty"`
}

// IsCTXDirective returns true if the key has a CTX_ prefix.
func IsCTXDirective(key string) bool {
	return strings.HasPrefix(key, "CTX_")
}

// IsKCTXDirective returns true if the key has a KCTX_ prefix (legacy/deprecated).
func IsKCTXDirective(key string) bool {
	return strings.HasPrefix(key, "KCTX_")
}

// MigrationError returns a clear error for bare KCTX_ entries pointing users to
// the new CTX_ syntax with a URI fragment.
func MigrationError(key, value string) error {
	group := strings.ToUpper(strings.TrimPrefix(key, "KCTX_"))
	return fmt.Errorf(
		"KCTX_ entries are no longer supported\n\n"+
			"Migrate %s=%s to the new syntax:\n\n"+
			"  CTX_%s=%s#KUBECONFIG\n\n"+
			"Then run: envoke resolve",
		key, value,
		group, value,
	)
}

// ParseContextEntry parses a CTX_<GROUP>=<uri>#<ENVVAR> raw entry.
// Returns an error if the fragment (#ENVVAR) is missing or malformed.
// Group name "meta" is valid here — rejection of "envoke switch meta" is in the switch command.
func ParseContextEntry(key, value string) (ContextEntry, error) {
	if !IsCTXDirective(key) {
		return ContextEntry{}, fmt.Errorf("not a CTX_ directive: %s", key)
	}
	group := strings.ToLower(strings.TrimPrefix(key, "CTX_"))
	if group == "" {
		return ContextEntry{}, fmt.Errorf("CTX_ directive has empty group name: %s", key)
	}

	hashIdx := strings.LastIndex(value, "#")
	if hashIdx < 0 || hashIdx == len(value)-1 {
		return ContextEntry{}, fmt.Errorf(
			"%s=%s is missing the #ENVVAR fragment\n\n"+
				"The fragment declares which env var the secret is written to:\n\n"+
				"  %s=%s#KUBECONFIG\n\n"+
				"See: envoke resolve --help",
			key, value, key, value,
		)
	}

	sourceURI := value[:hashIdx]
	envVar := value[hashIdx+1:]
	if envVar == "" {
		return ContextEntry{}, fmt.Errorf("%s=%s: env var name after # must not be empty", key, value)
	}
	if sourceURI == "" {
		return ContextEntry{}, fmt.Errorf("%s=%s: source URI before # must not be empty", key, value)
	}

	return ContextEntry{
		Group:     group,
		EnvVar:    envVar,
		SourceURI: sourceURI,
	}, nil
}

// ValidateContent validates secret content against built-in rules for known
// env var names. Unknown env var names get a no-op validator — content is
// written as-is without validation.
func ValidateContent(envVar string, content []byte) error {
	s := string(content)
	switch envVar {
	case "KUBECONFIG":
		if !strings.Contains(s, "apiVersion") {
			return fmt.Errorf("content for KUBECONFIG does not look like a kubeconfig (missing 'apiVersion')")
		}
	case "TALOSCONFIG":
		if !strings.Contains(s, "context:") {
			return fmt.Errorf("content for TALOSCONFIG does not look like a talosconfig (missing 'context:')")
		}
	case "AWS_SHARED_CREDENTIALS_FILE":
		if !strings.Contains(s, "[default]") && !strings.Contains(s, "[profile ") {
			return fmt.Errorf("content for AWS_SHARED_CREDENTIALS_FILE does not look like an AWS credentials file (missing '[default]' or '[profile ')")
		}
	case "DOCKER_CONFIG":
		var js interface{}
		if err := json.Unmarshal(content, &js); err != nil {
			return fmt.Errorf("content for DOCKER_CONFIG is not valid JSON: %w", err)
		}
	}
	// Unknown env var names: no validation, written as-is.
	return nil
}
