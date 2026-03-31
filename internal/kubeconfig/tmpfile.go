package kubeconfig

import (
	"fmt"
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

	f, err := os.CreateTemp(dir, prefix+"-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	if err := f.Chmod(0600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("chmod 600 %s: %w", f.Name(), err)
	}
	return f, nil
}
