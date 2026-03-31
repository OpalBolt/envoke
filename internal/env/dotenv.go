package env

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
)

// EnvEntry represents a single key-value pair from a .env file.
type EnvEntry struct {
	Key   string
	Value string // resolved value
	IsRef bool   // true if Value was a bw:// or vault:// reference
}

// ResolveDotEnv reads a .env file, batch-fetches needed BW folders,
// and returns resolved entries.
func ResolveDotEnv(path string, bwClient *secrets.BWClient, vaultClient *secrets.VaultClient) ([]EnvEntry, error) {
	lines, err := parseDotEnv(path)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Pass 1: collect unique bw:// folder/collection names for batch pre-fetch
	bwFolders := map[string]bool{}
	bwCollections := map[string]bool{}
	for _, e := range lines {
		if strings.HasPrefix(e.Value, "bw://") {
			ref, err := secrets.ParseBWRef(e.Value)
			if err == nil {
				if ref.IsCollection {
					bwCollections[ref.Folder] = true
				} else {
					bwFolders[ref.Folder] = true
				}
			}
		}
	}

	// Pre-fetch all BW folders (batch fetch — one unlock)
	for folder := range bwFolders {
		if _, err := bwClient.FolderItems(folder); err != nil {
			return nil, fmt.Errorf("pre-fetching BW folder %q: %w", folder, err)
		}
	}
	// Pre-fetch all BW collections
	for col := range bwCollections {
		if _, err := bwClient.CollectionItems(col); err != nil {
			return nil, fmt.Errorf("pre-fetching BW collection %q: %w", col, err)
		}
	}
	// Zero BW session token once all fetches are done
	defer bwClient.Close()

	// Pass 2: resolve all entries
	resolved := make([]EnvEntry, 0, len(lines))
	for _, e := range lines {
		entry := EnvEntry{Key: e.Key, Value: e.Value}
		if secrets.IsSecretRef(e.Value) {
			val, err := resolveRef(e.Value, bwClient, vaultClient)
			if err != nil {
				return nil, fmt.Errorf("resolving %s=%q: %w", e.Key, e.Value, err)
			}
			entry.Value = val
			entry.IsRef = true
		}
		resolved = append(resolved, entry)
	}
	return resolved, nil
}

// resolveRef resolves a single secret reference (bw:// or vault://).
func resolveRef(ref string, bwClient *secrets.BWClient, vaultClient *secrets.VaultClient) (string, error) {
	if strings.HasPrefix(ref, "bw://") {
		r, err := secrets.ParseBWRef(ref)
		if err != nil {
			return "", err
		}
		return bwClient.Resolve(r)
	}
	if strings.HasPrefix(ref, "vault://") {
		r, err := secrets.ParseVaultRef(ref)
		if err != nil {
			return "", err
		}
		return vaultClient.Resolve(r)
	}
	return ref, nil
}

// rawEntry is a pre-resolution key/value pair.
type rawEntry struct {
	Key   string
	Value string
}

// parseDotEnv reads a .env file and returns raw (key, value) pairs.
// Supports: KEY=value, KEY="value", KEY='value', export KEY=value
// Comments (#) and blank lines are ignored.
func parseDotEnv(path string) ([]rawEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []rawEntry
	scanner := bufio.NewScanner(f)
	// Increase buffer to 1 MiB — .env values can include PEM certs, kubeconfigs, JWTs
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue // no = sign, skip
		}
		key := strings.TrimSpace(line[:idx])
		val := line[idx+1:]
		val = unquote(val)
		entries = append(entries, rawEntry{Key: key, Value: val})
	}
	return entries, scanner.Err()
}

// unquote strips surrounding single or double quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
