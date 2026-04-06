package kubeconfig

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// NewTempFile creates a temporary file in /dev/shm if available, else /tmp.
// The file is chmod 600.
func NewTempFile(prefix string) (*os.File, error) {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		testPath := filepath.Join("/dev/shm", ".kctx-test")
		if f, err := os.OpenFile(testPath, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(testPath)
			dir = "/dev/shm"
		}
	}
	slog.Debug("creating temp file", "dir", dir, "prefix", prefix)

	f, err := os.CreateTemp(dir, prefix+"-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	if err := f.Chmod(0600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("chmod 600 %s: %w", f.Name(), err)
	}
	slog.Debug("created temp file", "path", f.Name())
	return f, nil
}

// IsManaged reports whether path looks like a kctx-managed kubeconfig tmpfile
// (i.e. a "kctx-" prefixed file in /dev/shm or /tmp).
func IsManaged(path string) bool {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if dir != "/dev/shm" && dir != "/tmp" {
		return false
	}
	return len(base) > 5 && base[:5] == "kctx-"
}

// ClearManaged removes all kctx-managed kubeconfig tmpfiles from /dev/shm and /tmp.
// Only files with the "kctx-" prefix are removed.
func ClearManaged() {
	for _, dir := range []string{"/dev/shm", "/tmp"} {
		matches, err := filepath.Glob(filepath.Join(dir, "kctx-*.tmp"))
		if err != nil {
			continue
		}
		for _, path := range matches {
			slog.Debug("cleanup: removing managed kubeconfig", "path", path)
			os.Remove(path)
		}
	}
}

// UnloadRequestFile returns the path of the sentinel file used to signal
// shells to run kctx unload on their next prompt draw.
func UnloadRequestFile(uid string) string {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		dir = "/dev/shm"
	}
	return filepath.Join(dir, "kctx-"+uid+"-unload-requested")
}

// RequestUnload creates the sentinel file. Shell PROMPT_COMMAND/precmd hooks
// installed by kctx shell-init check for this file and call kctx unload when
// they find it, unsetting KUBECONFIG from the shell on the next prompt.
func RequestUnload(uid string) error {
	path := UnloadRequestFile(uid)
	slog.Debug("requesting kctx shell unload via sentinel", "path", path)
	// Guard against symlink attacks: if the target path is already a symlink,
	// refuse to write rather than following the link to an unintended file.
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write unload sentinel %q: path is a symlink", path)
	}
	return os.WriteFile(path, []byte{}, 0600)
}
