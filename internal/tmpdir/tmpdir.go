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
// writable by creating and immediately removing a probe file named testName.
// If /dev/shm exists but is not writable it falls back to os.TempDir().
func PreferredWritable(testName string) string {
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		probe := filepath.Join("/dev/shm", testName)
		if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(probe)
			return "/dev/shm"
		}
	}
	return os.TempDir()
}

// Dirs returns the deduplicated list of directories to search when looking for
// files that may have been written by any version of envoke (which may have
// used either /dev/shm or os.TempDir()). Always contains at least one entry.
func Dirs() []string {
	tmpDir := os.TempDir()
	if tmpDir != "/dev/shm" {
		return []string{"/dev/shm", tmpDir}
	}
	return []string{"/dev/shm"}
}
