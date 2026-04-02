package env

import (
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
)

// validEnvKey matches POSIX env var names: start with letter or _, then alphanumeric or _.
var validEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// EmitExports writes "export KEY='value'" lines to w for each entry.
// Values are shell-quoted (single-quote escaped).
// Returns an error if any key contains characters that would break shell eval safety.
func EmitExports(w io.Writer, entries []EnvEntry) error {
	slog.Debug("emitting shell exports", "count", len(entries))
	for _, e := range entries {
		if !validEnvKey.MatchString(e.Key) {
			slog.Warn("skipping invalid env key", "key", e.Key)
			return fmt.Errorf("invalid env key %q: must match [A-Za-z_][A-Za-z0-9_]*", e.Key)
		}
		quoted := shellQuote(e.Value)
		if _, err := fmt.Fprintf(w, "export %s=%s\n", e.Key, quoted); err != nil {
			return err
		}
	}
	return nil
}

// EmitUnload writes "unset KEY" lines to w for each entry.
func EmitUnload(w io.Writer, entries []EnvEntry) error {
	slog.Debug("emitting shell unsets", "count", len(entries))
	for _, e := range entries {
		if !validEnvKey.MatchString(e.Key) {
			return fmt.Errorf("invalid env key %q: must match [A-Za-z_][A-Za-z0-9_]*", e.Key)
		}
		if _, err := fmt.Fprintf(w, "unset %s\n", e.Key); err != nil {
			return err
		}
	}
	return nil
}

// shellQuote returns s wrapped in single quotes, with internal single quotes escaped.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
