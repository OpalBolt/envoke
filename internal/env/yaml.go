package env

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
	"gopkg.in/yaml.v3"
)

// ResolveYAML reads a YAML file, walks all scalar string values, resolves
// any bw:// or vault:// references, and returns the resolved data structure.
func ResolveYAML(path string, bwClient *secrets.BWClient, vaultClient *secrets.VaultClient) (interface{}, error) {
	slog.Debug("reading YAML file", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	result, err := ResolveYAMLString(string(data), bwClient, vaultClient)
	if err != nil {
		return nil, err
	}
	slog.Info("resolved YAML file", "path", path)
	return result, nil
}

// ResolveYAMLString resolves a YAML string.
func ResolveYAMLString(yamlStr string, bwClient *secrets.BWClient, vaultClient *secrets.VaultClient) (interface{}, error) {
	var doc interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	resolved, err := walkAndResolve(doc, bwClient, vaultClient)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// walkAndResolve recursively walks a YAML value and resolves secret refs.
func walkAndResolve(v interface{}, bwClient *secrets.BWClient, vaultClient *secrets.VaultClient) (interface{}, error) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			resolved, err := walkAndResolve(child, bwClient, vaultClient)
			if err != nil {
				return nil, err
			}
			val[k] = resolved
		}
		return val, nil
	case []interface{}:
		for i, child := range val {
			resolved, err := walkAndResolve(child, bwClient, vaultClient)
			if err != nil {
				return nil, err
			}
			val[i] = resolved
		}
		return val, nil
	case string:
		if secrets.IsSecretRef(val) {
			slog.Debug("resolving YAML secret ref", "ref", val)
			if strings.HasPrefix(val, "bw://") {
				ref, err := secrets.ParseBWRef(val)
				if err != nil {
					return nil, err
				}
				return bwClient.Resolve(ref)
			}
			if strings.HasPrefix(val, "vault://") {
				ref, err := secrets.ParseVaultRef(val)
				if err != nil {
					return nil, err
				}
				return vaultClient.Resolve(ref)
			}
		}
		return val, nil
	default:
		return v, nil
	}
}

// MarshalYAML marshals a value to YAML bytes.
func MarshalYAML(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// YAMLLookup looks up a dot-notation key path in a YAML document.
// Supports list indices (e.g. "list.0.field").
func YAMLLookup(doc interface{}, key string) (string, error) {
	parts := strings.Split(key, ".")
	cur := doc
	for _, part := range parts {
		switch v := cur.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return "", fmt.Errorf("key %q not found", part)
			}
			cur = val
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil {
				return "", fmt.Errorf("expected integer index, got %q", part)
			}
			if idx < 0 || idx >= len(v) {
				return "", fmt.Errorf("index %d out of range [0, %d)", idx, len(v))
			}
			cur = v[idx]
		default:
			return "", fmt.Errorf("cannot traverse %T at %q", cur, part)
		}
	}
	return fmt.Sprintf("%v", cur), nil
}
