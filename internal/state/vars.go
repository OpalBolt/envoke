package state

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/opalbolt/envoke/internal/securedir"
)

// varsFilePath returns the path of the tracked-vars state file for uid.
func varsFilePath(uid string) string {
	return filepath.Join(securedir.Dir(), "renv-"+uid+"-vars")
}

// SaveVarNames writes the exported variable names to a state file so that
// renv unload can emit the correct unset commands later.
func SaveVarNames(uid string, names []string) error {
	path := varsFilePath(uid)
	slog.Debug("saving tracked var names", "path", path, "count", len(names))
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
		slog.Debug("no tracked var names found", "path", path)
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
	slog.Debug("loaded tracked var names", "path", path, "count", len(names))
	return names, nil
}

// ClearVarNames removes the tracked-vars state file for uid.
func ClearVarNames(uid string) error {
	path := varsFilePath(uid)
	slog.Debug("clearing tracked var names", "path", path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing vars state: %w", err)
	}
	return nil
}

// UnloadRequestFile returns the path of the sentinel file used to signal
// shells to run envoke unload on their next prompt draw.
func UnloadRequestFile(uid string) string {
	return filepath.Join(securedir.Dir(), "envoke-"+uid+"-unload-requested")
}

// RequestUnload creates the sentinel file. Shell PROMPT_COMMAND/precmd hooks
// installed by renv shell-init check for this file and call renv unload when
// they find it, clearing secret variables from the shell on the next prompt.
func RequestUnload(uid string) error {
	path := UnloadRequestFile(uid)
	slog.Debug("requesting shell unload via sentinel", "path", path)
	// Guard against symlink attacks: if the target path is already a symlink,
	// refuse to write rather than following the link to an unintended file.
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write unload sentinel %q: path is a symlink", path)
	}
	return os.WriteFile(path, []byte{}, 0600)
}
