package kubeconfig

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/opalbolt/envoke/internal/securedir"
)

const (
	storeFilePerms = 0600
	storePrefix    = "kctx-kc-"
)

// validStoreName matches safe kubeconfig names (alphanumeric, dash, underscore, dot).
var validStoreName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// NamedStore stores named kubeconfig data as plaintext files.
// Files are stored as kctx-kc-<uid>-<name>.yaml in /dev/shm (preferred) or /tmp.
type NamedStore struct {
	Dir    string
	MaxAge time.Duration
}

// NewNamedStore uses the platform-appropriate secure directory (see internal/securedir).
func NewNamedStore() *NamedStore {
	return &NamedStore{Dir: securedir.Dir(), MaxAge: 8 * time.Hour}
}

// ValidateStoreName returns an error if name is empty or contains unsafe characters.
func ValidateStoreName(name string) error {
	if name == "" {
		return fmt.Errorf("kubeconfig name must not be empty")
	}
	if !validStoreName.MatchString(name) {
		return fmt.Errorf("invalid kubeconfig name %q: only [a-zA-Z0-9_.-] are allowed", name)
	}
	return nil
}

// storePath returns the on-disk path for a named kubeconfig.
func (s *NamedStore) storePath(uid, name string) string {
	return filepath.Join(s.Dir, storePrefix+uid+"-"+name+".yaml")
}

// Path returns the on-disk path for a named kubeconfig.
// The file may or may not exist yet.
func (s *NamedStore) Path(uid, name string) string {
	return s.storePath(uid, name)
}

// Put writes the kubeconfig data as plaintext to a file for uid/name.
// Existing entries with the same name are overwritten.
// Uses atomic write (tmp file + rename) for safety.
func (s *NamedStore) Put(uid, name string, data []byte) error {
	if err := ValidateStoreName(name); err != nil {
		return err
	}

	path := s.storePath(uid, name)
	slog.Debug("named store put", "path", path, "name", name)

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("store: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(storeFilePerms); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: setting temp file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("store: replacing file atomically: %w", err)
	}
	cleanupTmp = false
	return nil
}

// Get returns the stored kubeconfig for uid/name.
// Returns (nil, nil) on cache miss or expiry.
func (s *NamedStore) Get(uid, name string) ([]byte, error) {
	if err := ValidateStoreName(name); err != nil {
		return nil, err
	}
	path := s.storePath(uid, name)
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		slog.Debug("named store miss (not found)", "name", name)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: stat: %w", err)
	}
	if s.MaxAge > 0 && time.Since(fi.ModTime()) > s.MaxAge {
		slog.Debug("named store expired", "name", name, "age", time.Since(fi.ModTime()).Round(time.Second))
		os.Remove(path)
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("store: reading file: %w", err)
	}
	return data, nil
}

// List returns the names of all non-expired named kubeconfigs for uid.
func (s *NamedStore) List(uid string) ([]string, error) {
	pattern := filepath.Join(s.Dir, storePrefix+uid+"-*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("store: glob: %w", err)
	}
	prefix := storePrefix + uid + "-"
	var names []string
	for _, m := range matches {
		base := filepath.Base(m)
		name := strings.TrimPrefix(base, prefix)
		name = strings.TrimSuffix(name, ".yaml")
		if !validStoreName.MatchString(name) {
			continue
		}
		if s.MaxAge > 0 {
			if fi, err := os.Stat(m); err == nil && time.Since(fi.ModTime()) > s.MaxAge {
				os.Remove(m)
				continue
			}
		}
		names = append(names, name)
	}
	return names, nil
}

// Remove deletes the named kubeconfig for uid. No error if it does not exist.
func (s *NamedStore) Remove(uid, name string) error {
	if err := ValidateStoreName(name); err != nil {
		return err
	}
	path := s.storePath(uid, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("store: removing %s: %w", path, err)
	}
	return nil
}

// Clear removes all named kubeconfigs for uid.
func (s *NamedStore) Clear(uid string) error {
	pattern := filepath.Join(s.Dir, storePrefix+uid+"-*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("store: glob: %w", err)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: removing %s: %w", m, err)
		}
	}
	return nil
}
