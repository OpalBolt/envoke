package env

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/opalbolt/envoke/internal/providers"
)

// EnvEntry represents a single key-value pair from a .env file.
type EnvEntry struct {
	Key    string
	Value  string // resolved value
	IsRef  bool   // true if Value was a bw:// or vault:// reference
	Source string // original reference URI (e.g. "bw://folder/item" or "vault://path#field"); empty for literals
}

// ResolveDotEnv reads a .env file, resolves all secret references via the
// provided Registry, and returns the resolved entries.
//
// The registry pre-warms on the first Resolve call per backend (e.g. BW unlock
// and disk cache fill happen on the first bw:// reference encountered); subsequent
// references to the same folder reuse the in-process session and cached data.
func ResolveDotEnv(path string, reg *providers.Registry) ([]EnvEntry, error) {
	slog.Debug("parsing .env file", "path", path)
	lines, err := parseDotEnv(path)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	slog.Debug("parsed .env file", "path", path, "entries", len(lines))

	resolved := make([]EnvEntry, 0, len(lines))
	refCount := 0
	for _, e := range lines {
		entry := EnvEntry{Key: e.Key, Value: e.Value}
		if reg.IsSecretRef(e.Value) {
			slog.Debug("resolving secret ref", "key", e.Key, "ref", e.Value)
			val, err := reg.Resolve(e.Value)
			if err != nil {
				return nil, fmt.Errorf("resolving %s=%q: %w", e.Key, e.Value, err)
			}
			entry.Value = val
			entry.IsRef = true
			entry.Source = e.Value
			refCount++
		}
		resolved = append(resolved, entry)
	}
	slog.Info("resolved .env file", "path", path, "entries", len(resolved), "secrets", refCount)
	return resolved, nil
}

// rawEntry is a pre-resolution key/value pair.
type rawEntry struct {
	Key   string
	Value string
}

// RawEntry is an exported pre-resolution key/value pair from a .env file.
type RawEntry struct {
	Key   string
	Value string
}

// ParseRaw reads a .env file and returns the raw (unresolved) key-value pairs.
// Useful when callers need to inspect or partition entries before resolution.
func ParseRaw(path string) ([]RawEntry, error) {
	entries, err := parseDotEnv(path)
	if err != nil {
		return nil, err
	}
	out := make([]RawEntry, len(entries))
	for i, e := range entries {
		out[i] = RawEntry{Key: e.Key, Value: e.Value}
	}
	return out, nil
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
