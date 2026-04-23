//go:build darwin

package securedir

import (
	"os"
	"path/filepath"
)

// Dir returns the most-secure writable directory for session-scoped files on macOS.
//
// Priority:
//  1. os.UserCacheDir()/envoke — user-owned, outside the shared /tmp.
//  2. os.TempDir() — returns /var/folders/…/T/ on macOS, which is user-owned
//     and mode 0700; better than the shared /tmp.
//  3. /tmp — universal fallback.
//
// Note: os.TempDir() already respects $TMPDIR on macOS, so this also satisfies
// the $TMPDIR/$TEMPDIR env-var feature (#26).
func Dir() string {
	if cacheDir, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(cacheDir, "envoke")
		if err := os.MkdirAll(dir, 0700); err == nil {
			probe := filepath.Join(dir, ".envoke-probe")
			if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
				f.Close()
				os.Remove(probe)
				return dir
			}
		}
	}

	// os.TempDir() on macOS returns /var/folders/…/T/ — user-owned, 0700.
	if tmp := os.TempDir(); tmp != "/tmp" && tmp != "" {
		probe := filepath.Join(tmp, ".envoke-probe")
		if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(probe)
			return tmp
		}
	}

	return "/tmp"
}
