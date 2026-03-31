package env

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

// validEnvKey matches POSIX env var names: start with letter or _, then alphanumeric or _.
var validEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// EmitExports writes "export KEY='value'" lines to w for each entry.
// Values are shell-quoted (single-quote escaped).
// Returns an error if any key contains characters that would break shell eval safety.
func EmitExports(w io.Writer, entries []EnvEntry) error {
	for _, e := range entries {
		if !validEnvKey.MatchString(e.Key) {
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
	for _, e := range entries {
		if _, err := fmt.Fprintf(w, "unset %s\n", e.Key); err != nil {
			return err
		}
	}
	return nil
}

// ShellIntegrationSnippet returns shell function source for users to source into their shell.
func ShellIntegrationSnippet() string {
	return `
# renv shell integration — source this into your shell
# Usage: source <(renv shell-init)

resolve_env_file() {
  local file="${1:-.env}"
  local exports
  exports="$(renv resolve --file "$file")" || return 1
  eval "$exports"
  # Register EXIT trap to unload variables
  trap 'unload_env' EXIT
}

unload_env() {
  eval "$(renv unload)"
}
`
}

// shellQuote returns s wrapped in single quotes, with internal single quotes escaped.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
