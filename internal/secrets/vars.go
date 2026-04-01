package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// varsFilePath returns the path of the tracked-vars state file for uid.
// Prefers /dev/shm (RAM-backed, cleared on reboot) over /tmp.
func varsFilePath(uid string) string {
	dir := "/tmp"
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		dir = "/dev/shm"
	}
	return filepath.Join(dir, "renv-"+uid+"-vars")
}

// SaveVarNames writes the exported variable names to a state file so that
// renv unload can emit the correct unset commands later.
func SaveVarNames(uid string, names []string) error {
	path := varsFilePath(uid)
	content := strings.Join(names, "\n")
	if len(names) > 0 {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0600)
}

// LoadVarNames reads the tracked variable names from the state file.
// Returns a nil slice (and no error) if the file does not exist.
func LoadVarNames(uid string) ([]string, error) {
	path := varsFilePath(uid)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading vars state: %w", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// ClearVarNames removes the tracked-vars state file for uid.
func ClearVarNames(uid string) error {
	path := varsFilePath(uid)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing vars state: %w", err)
	}
	return nil
}
