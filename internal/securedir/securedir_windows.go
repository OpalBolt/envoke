//go:build windows

package securedir

import (
	"os"
	"path/filepath"
)

// Dir returns the most-secure writable directory for session-scoped files on Windows.
//
// Uses %LOCALAPPDATA%\envoke\secure — per-user and more predictably located than
// %TEMP%. The directory is created with user-only permissions (0700 approximation
// via os.MkdirAll; full ACL restriction is a future improvement).
// Falls back to os.TempDir() if LOCALAPPDATA is unavailable.
func Dir() string {
	if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
		dir := filepath.Join(localApp, "envoke", "secure")
		if err := os.MkdirAll(dir, 0700); err == nil {
			return dir
		}
	}
	return os.TempDir()
}
