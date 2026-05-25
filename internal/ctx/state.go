package ctx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opalbolt/envoke/internal/securedir"
)

// GroupsState persists the full CTX group resolution across envoke subcommands.
// It is written to securedir after `envoke resolve` and read by `envoke switch`,
// `envoke status`, and `envoke unload` — no provider calls are needed after resolve.
type GroupsState struct {
	// Groups maps lowercase group name → ordered slice of context entries.
	// The META baseline is stored under the reserved key "meta".
	Groups map[string][]ContextEntry `json:"groups"`
	// ActiveGroup is the group currently exported into the shell session.
	// Empty string means no group has been switched to yet.
	ActiveGroup string `json:"active_group"`
}

// statePath returns the on-disk path for the CTX state file.
func statePath(uid string) string {
	return filepath.Join(securedir.Dir(), "envoke-"+uid+"-ctx-state.json")
}

// SaveState writes the full group state to disk with 0600 permissions.
func SaveState(uid string, st GroupsState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("ctx state: marshal: %w", err)
	}
	path := statePath(uid)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("ctx state: write %s: %w", path, err)
	}
	return nil
}

// LoadState reads the group state from disk.
// Returns an empty GroupsState (and no error) if the file does not exist yet.
func LoadState(uid string) (GroupsState, error) {
	path := statePath(uid)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return GroupsState{Groups: make(map[string][]ContextEntry)}, nil
	}
	if err != nil {
		return GroupsState{}, fmt.Errorf("ctx state: read %s: %w", path, err)
	}
	var st GroupsState
	if err := json.Unmarshal(data, &st); err != nil {
		return GroupsState{}, fmt.Errorf("ctx state: unmarshal: %w", err)
	}
	if st.Groups == nil {
		st.Groups = make(map[string][]ContextEntry)
	}
	return st, nil
}

// ClearState removes the state file and all tmpfiles referenced in it.
// Called by `envoke unload`. Best-effort on individual tmpfile removal.
func ClearState(uid string) error {
	st, err := LoadState(uid)
	if err != nil {
		// Best-effort: just remove the state file even if we can't parse it.
		_ = os.Remove(statePath(uid))
		return nil
	}
	for _, entries := range st.Groups {
		for _, e := range entries {
			if e.TmpfilePath != "" {
				_ = os.Remove(e.TmpfilePath)
			}
		}
	}
	path := statePath(uid)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ctx state: remove %s: %w", path, err)
	}
	return nil
}

// SetActiveGroup updates only the active group field in the state file.
// Used by switchCmd to track which group is currently exported.
func SetActiveGroup(uid, group string) error {
	st, err := LoadState(uid)
	if err != nil {
		return err
	}
	st.ActiveGroup = group
	return SaveState(uid, st)
}
