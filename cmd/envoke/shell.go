package main

import (
	"io"

	"github.com/spf13/cobra"
)

func shellInitCmd() *cobra.Command {
	var shell string

	cmd := &cobra.Command{
		Use:   "shell-init",
		Short: "Print shell functions for both renv and kctx",
		Long: `Print shell function definitions for both renv and kctx, wired to call
the envoke binary. Only the envoke binary needs to be in PATH.

Add to your shell config once:

  # bash / zsh (~/.bashrc or ~/.zshrc)
  eval "$(envoke shell-init)"

  # fish (~/.config/fish/config.fish)
  envoke shell-init --shell fish | source

After that:
  renv resolve .env          # load env secrets
  kctx prod                  # switch to 'prod' kubeconfig
  envoke resolve .env        # load both secrets and kubeconfigs`,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch shell {
			case "fish":
				_, err := io.WriteString(cmd.OutOrStdout(), fishCombinedInitScript)
				return err
			default:
				_, err := io.WriteString(cmd.OutOrStdout(), bashCombinedInitScript)
				return err
			}
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type: bash, zsh, fish")
	return cmd
}

// bashCombinedInitScript is the combined shell snippet for bash/zsh.
// Defines renv() and kctx() functions that call `envoke renv ...` / `envoke kctx ...`,
// starts a single combined watcher, and installs a combined EXIT trap.
const bashCombinedInitScript = `
# envoke shell integration — add to ~/.bashrc or ~/.zshrc:
#   eval "$(envoke shell-init)"
#
# This sets up renv and kctx shell functions backed by the single envoke binary.

renv() {
  case "$1" in
    resolve|unload)
      # Strip the standalone EXIT trap — the combined trap below covers cleanup.
      eval "$(command envoke renv "$@" | grep -v '^trap ')"
      ;;
    *)
      command envoke renv "$@"
      ;;
  esac
}

kctx() {
  case "$1" in
    load)
      command envoke kctx load "${@:2}"
      ;;
    switch)
      # IMPORTANT: never call 'trap' inside this function. In zsh, a trap set
      # inside a function fires when the function returns, not when the shell
      # exits. Kubeconfig cleanup is handled by the shell-init EXIT trap below.
      if _kctx_raw="$(command envoke kctx switch "${@:2}")"; then
        eval "$(echo "$_kctx_raw" | grep -v '^trap ')"
        _KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
      fi
      ;;
    unload)
      eval "$(command envoke kctx unload)"
      ;;
    status|clear-cache|watch|shell-init)
      command envoke kctx "$@"
      ;;
    --version|--help|-h)
      command envoke kctx "$@"
      ;;
    "")
      command envoke kctx
      ;;
    *)
      # Positional shorthand: kctx prod → kctx switch prod
      if _kctx_raw="$(command envoke kctx switch "$@")"; then
        eval "$(echo "$_kctx_raw" | grep -v '^trap ')"
        _KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
      fi
      ;;
  esac
}

envoke() {
  case "$1" in
    resolve)
      # Help/version: print directly, do not eval — scan all args.
      local _a
      for _a in "${@:2}"; do
        case "$_a" in --help|-h|--version) command envoke resolve "${@:2}"; return ;; esac
      done
      # Capture output and exit code before eval so failures propagate correctly.
      local _envoke_out _envoke_exit
      _envoke_out="$(command envoke resolve "${@:2}")"
      _envoke_exit=$?
      [ "$_envoke_exit" -ne 0 ] && return "$_envoke_exit"
      # Auto-eval so secrets and kubeconfigs are loaded into the current shell.
      # Strip the standalone EXIT trap — the shell-init trap below covers cleanup.
      eval "$(printf '%s\n' "$_envoke_out" | grep -v '^trap ')"
      ;;
    unload)
      # Help/version: print directly, do not eval — scan all args.
      local _a
      for _a in "${@:2}"; do
        case "$_a" in --help|-h|--version) command envoke unload "${@:2}"; return ;; esac
      done
      # Capture output and exit code before eval so failures propagate correctly.
      local _envoke_out _envoke_exit
      _envoke_out="$(command envoke unload)"
      _envoke_exit=$?
      [ "$_envoke_exit" -ne 0 ] && return "$_envoke_exit"
      eval "$(printf '%s\n' "$_envoke_out" | grep -v '^trap ')"
      ;;
    renv)
      # Delegate to the renv shell function which handles eval internally.
      renv "${@:2}"
      ;;
    kctx)
      # Delegate to the kctx shell function which handles eval internally.
      kctx "${@:2}"
      ;;
    *)
      command envoke "$@"
      ;;
  esac
}

# ── renv unload token ──────────────────────────────────────────────────────────
_renv_unload_token() {
  local f="/dev/shm/renv-${UID}-unload-requested"
  [ -f "$f" ] || f="/tmp/renv-${UID}-unload-requested"
  [ -f "$f" ] || return 1
  stat -c '%Y:%i:%s' "$f" 2>/dev/null || stat -f '%m:%i:%z' "$f" 2>/dev/null
}

_renv_check_unload() {
  local token
  token="$(_renv_unload_token)" || return 0
  [ "${_RENV_LAST_UNLOAD_TOKEN:-}" = "$token" ] && return 0
  _RENV_LAST_UNLOAD_TOKEN="$token"
  eval "$(command envoke renv unload 2>/dev/null)" 2>/dev/null || true
}
_RENV_LAST_UNLOAD_TOKEN="$(_renv_unload_token 2>/dev/null || true)"

# ── kctx unload token ──────────────────────────────────────────────────────────
_kctx_unload_token() {
  local f="/dev/shm/kctx-${UID}-unload-requested"
  [ -f "$f" ] || f="/tmp/kctx-${UID}-unload-requested"
  [ -f "$f" ] || return 1
  stat -c '%Y:%i:%s' "$f" 2>/dev/null || stat -f '%m:%i:%z' "$f" 2>/dev/null
}

_kctx_check_unload() {
  local token
  token="$(_kctx_unload_token)" || return 0
  [ "${_KCTX_LAST_UNLOAD_TOKEN:-}" = "$token" ] && return 0
  _KCTX_LAST_UNLOAD_TOKEN="$token"
  eval "$(command envoke kctx unload 2>/dev/null)" 2>/dev/null || true
}
_KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"

# ── install prompt hooks ───────────────────────────────────────────────────────
if [ -n "${ZSH_VERSION:-}" ]; then
  autoload -Uz add-zsh-hook 2>/dev/null
  add-zsh-hook precmd _renv_check_unload
  add-zsh-hook precmd _kctx_check_unload
else
  PROMPT_COMMAND="_renv_check_unload; _kctx_check_unload${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
fi

# ── combined watcher + EXIT trap ───────────────────────────────────────────────
if [ -z "${_ENVOKE_WATCH_PID:-}" ]; then
  command envoke watch &
  _ENVOKE_WATCH_PID=$!
  trap 'eval "$(command envoke unload 2>/dev/null || true)"; kill "${_ENVOKE_WATCH_PID:-}" 2>/dev/null; command envoke clear-cache 2>/dev/null' EXIT
fi
`

// fishCombinedInitScript is the combined shell snippet for fish.
const fishCombinedInitScript = `
# envoke shell integration for fish — add to ~/.config/fish/config.fish:
#   envoke shell-init --shell fish | source

function renv
  switch $argv[1]
    case resolve unload
      command envoke renv $argv | source
    case '*'
      command envoke renv $argv
  end
end

function kctx
  switch $argv[1]
    case load
      command envoke kctx load $argv[2..]
    case switch
      set -l _kctx_raw (command envoke kctx switch $argv[2..] 2>/dev/null)
      if test $status -eq 0
        echo $_kctx_raw | grep -v '^trap ' | source
        set -g _KCTX_LAST_UNLOAD_TOKEN (_kctx_unload_token 2>/dev/null; or echo "")
      end
    case unload
      command envoke kctx unload | source
    case status clear-cache watch shell-init
      command envoke kctx $argv
    case ''
      command envoke kctx
    case '*'
      set -l _kctx_raw (command envoke kctx switch $argv 2>/dev/null)
      if test $status -eq 0
        echo $_kctx_raw | grep -v '^trap ' | source
        set -g _KCTX_LAST_UNLOAD_TOKEN (_kctx_unload_token 2>/dev/null; or echo "")
      end
  end
end

function envoke
  switch $argv[1]
    case resolve
      # Help/version: print directly, do not source.
      if contains -- --help $argv; or contains -- -h $argv
        command envoke resolve $argv[2..]
        return
      end
      # Capture output and exit code before sourcing so failures propagate correctly.
      set -l _envoke_out (command envoke resolve $argv[2..])
      set -l _envoke_exit $status
      test $_envoke_exit -ne 0; and return $_envoke_exit
      # Auto-source so secrets and kubeconfigs are loaded into the current shell.
      printf '%s\n' $_envoke_out | grep -v '^trap ' | source
    case unload
      # Help/version: print directly, do not source.
      if contains -- --help $argv; or contains -- -h $argv
        command envoke unload $argv[2..]
        return
      end
      # Capture output and exit code before sourcing so failures propagate correctly.
      set -l _envoke_out (command envoke unload)
      set -l _envoke_exit $status
      test $_envoke_exit -ne 0; and return $_envoke_exit
      printf '%s\n' $_envoke_out | grep -v '^trap ' | source
    case renv
      renv $argv[2..]
    case kctx
      kctx $argv[2..]
    case '*'
      command envoke $argv
  end
end

function _renv_unload_token
  set -l f /dev/shm/renv-(id -u)-unload-requested
  test -f $f; or set f /tmp/renv-(id -u)-unload-requested
  test -f $f; or return 1
  stat -c '%Y:%i:%s' $f 2>/dev/null; or stat -f '%m:%i:%z' $f 2>/dev/null
end

function _kctx_unload_token
  set -l f /dev/shm/kctx-(id -u)-unload-requested
  test -f $f; or set f /tmp/kctx-(id -u)-unload-requested
  test -f $f; or return 1
  stat -c '%Y:%i:%s' $f 2>/dev/null; or stat -f '%m:%i:%z' $f 2>/dev/null
end

function _envoke_check_unload --on-event fish_prompt
  set -l rtoken (_renv_unload_token 2>/dev/null); or set rtoken ""
  if test "$_RENV_LAST_UNLOAD_TOKEN" != "$rtoken"
    set -g _RENV_LAST_UNLOAD_TOKEN $rtoken
    command envoke renv unload | source 2>/dev/null; or true
  end
  set -l ktoken (_kctx_unload_token 2>/dev/null); or set ktoken ""
  if test "$_KCTX_LAST_UNLOAD_TOKEN" != "$ktoken"
    set -g _KCTX_LAST_UNLOAD_TOKEN $ktoken
    command envoke kctx unload | source 2>/dev/null; or true
  end
end
set -g _RENV_LAST_UNLOAD_TOKEN (_renv_unload_token 2>/dev/null; or echo "")
set -g _KCTX_LAST_UNLOAD_TOKEN (_kctx_unload_token 2>/dev/null; or echo "")

if not set -q _ENVOKE_WATCH_PID
  command envoke watch &
  set -gx _ENVOKE_WATCH_PID $last_pid
end

function _envoke_cleanup --on-event fish_exit
  command envoke unload 2>/dev/null | source 2>/dev/null; or true
  if set -q _ENVOKE_WATCH_PID
    kill $_ENVOKE_WATCH_PID 2>/dev/null; or true
    set -e _ENVOKE_WATCH_PID
  end
  command envoke clear-cache 2>/dev/null; or true
end
`
