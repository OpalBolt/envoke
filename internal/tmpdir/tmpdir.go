// Package tmpdir provides helpers for selecting a temporary directory.
// It prefers /dev/shm (Linux RAM-backed tmpfs) when available, and falls back
// to os.TempDir(), which honours $TMPDIR on Unix (and equivalent platform env
// vars on other operating systems).
package tmpdir

import (
	"os"
	"path/filepath"
)

// Preferred returns the best available temporary directory.
// Priority:
//  1. /dev/shm — Linux RAM-backed tmpfs (cleared on reboot, never swapped)
//  2. os.TempDir() — respects $TMPDIR on Unix / macOS, %TEMP% on Windows
func Preferred() string {
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		return "/dev/shm"
	}
	return os.TempDir()
}

// PreferredWritable is like Preferred but also verifies the directory is
// writable by creating and immediately removing a uniquely named probe file.
// patternHint is an optional base name used to customise the probe filename
// pattern (e.g. ".envoke-cache-test" → ".envoke-cache-test-*").
// If /dev/shm exists but is not writable it falls back to os.TempDir().
func PreferredWritable(patternHint string) string {
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		pattern := "tmpdir-probe-*"
		if base := filepath.Base(patternHint); base != "" && base != "." {
			pattern = base + "-*"
		}
		if f, err := os.CreateTemp("/dev/shm", pattern); err == nil {
			name := f.Name()
			f.Close()
			os.Remove(name)
			return "/dev/shm"
		}
	}
	return os.TempDir()
}

// Dirs returns the deduplicated list of directories to search when looking for
// envoke files (which may have been written to /dev/shm or os.TempDir()).
// Always contains at least one entry.
func Dirs() []string {
	candidates := []string{"/dev/shm", os.TempDir()}
	dirs := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))

	for _, dir := range candidates {
		cleaned := filepath.Clean(dir)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		dirs = append(dirs, cleaned)
	}

	if len(dirs) == 0 {
		dirs = append(dirs, os.TempDir())
	}
	return dirs
}
