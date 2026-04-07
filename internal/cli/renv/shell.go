package renv

import (
	"io"

	"github.com/spf13/cobra"
)

// BashInitScript is the shell function emitted by `renv shell-init` for bash/zsh.
// Exported so envoke can reference it when building the combined shell-init.
const BashInitScript = `renv() {
  case "$1" in
    resolve|unload)
      # Strip the standalone EXIT trap emitted by resolve — the shell-init
      # trap below covers cache clear and watcher shutdown together.
      eval "$(command renv "$@" | grep -v '^trap ')"
      ;;
    *)
      command renv "$@"
      ;;
  esac
}

# Return a token that changes whenever the unload sentinel is refreshed.
_renv_unload_token() {
  local f="/dev/shm/renv-${UID}-unload-requested"
  [ -f "$f" ] || f="/tmp/renv-${UID}-unload-requested"
  [ -f "$f" ] || return 1
  stat -c '%Y:%i:%s' "$f" 2>/dev/null || stat -f '%m:%i:%z' "$f" 2>/dev/null
}

# Unload secret variables when the watcher signals that sleep/lock occurred.
_renv_check_unload() {
  local token
  token="$(_renv_unload_token)" || return 0
  [ "${_RENV_LAST_UNLOAD_TOKEN:-}" = "$token" ] && return 0
  _RENV_LAST_UNLOAD_TOKEN="$token"
  eval "$(command renv unload 2>/dev/null)" 2>/dev/null || true
}
_RENV_LAST_UNLOAD_TOKEN="$(_renv_unload_token 2>/dev/null || true)"
if [ -n "${ZSH_VERSION:-}" ]; then
  autoload -Uz add-zsh-hook 2>/dev/null && add-zsh-hook precmd _renv_check_unload
else
  PROMPT_COMMAND="_renv_check_unload${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
fi

# Start the sleep/lock watcher once per shell session.
if [ -z "${_RENV_WATCH_PID:-}" ]; then
  command renv watch &
  _RENV_WATCH_PID=$!
  trap 'kill "${_RENV_WATCH_PID:-}" 2>/dev/null; command renv clear-cache 2>/dev/null' EXIT
fi
`

// FishInitScript is the shell function emitted by `renv shell-init --shell fish`.
const FishInitScript = `function renv
  switch $argv[1]
    case resolve unload
      command renv $argv | source
    case '*'
      command renv $argv
  end
end

function _renv_unload_token
  set -l f /dev/shm/renv-(id -u)-unload-requested
  test -f $f; or set f /tmp/renv-(id -u)-unload-requested
  test -f $f; or return 1
  stat -c '%Y:%i:%s' $f 2>/dev/null; or stat -f '%m:%i:%z' $f 2>/dev/null
end

function _renv_check_unload --on-event fish_prompt
  set -l token (_renv_unload_token 2>/dev/null); or return
  test "$_RENV_LAST_UNLOAD_TOKEN" = "$token"; and return
  set -g _RENV_LAST_UNLOAD_TOKEN $token
  command renv unload | source 2>/dev/null; or true
end
set -g _RENV_LAST_UNLOAD_TOKEN (_renv_unload_token 2>/dev/null; or echo "")

if not set -q _RENV_WATCH_PID
  command renv watch &
  set -gx _RENV_WATCH_PID $last_pid
end

function _renv_cleanup --on-event fish_exit
  if set -q _RENV_WATCH_PID
    kill $_RENV_WATCH_PID 2>/dev/null; or true
    set -e _RENV_WATCH_PID
  end
  command renv clear-cache 2>/dev/null; or true
end
`

func shellInitCmd() *cobra.Command {
	var shell string

	cmd := &cobra.Command{
		Use:   "shell-init",
		Short: "Print shell function definition so renv resolve works without eval",
		Long: `Print a shell function definition that wraps renv resolve and renv unload
with eval, so you never have to type it yourself.

Add to your shell config once:

  # bash / zsh (~/.bashrc or ~/.zshrc)
  eval "$(renv shell-init)"

  # fish (~/.config/fish/config.fish)
  renv shell-init --shell fish | source

After that, renv resolve .env and renv unload work without explicit eval.
All other renv subcommands (exec, yaml, status, …) pass through unchanged.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch shell {
			case "fish":
				_, err := io.WriteString(cmd.OutOrStdout(), FishInitScript)
				return err
			default:
				_, err := io.WriteString(cmd.OutOrStdout(), BashInitScript)
				return err
			}
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type: bash, zsh, fish")
	return cmd
}
