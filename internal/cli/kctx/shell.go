package kctx

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ShellSnippet returns the kctx bash/zsh shell init snippet.
// Exported so envoke can reference it when building the combined shell-init.
func ShellSnippet() string {
	return `
# kctx shell integration — add to ~/.bashrc or ~/.zshrc:
# eval "$(kctx shell-init)"

kctx() {
  case "$1" in
    load)
      command kctx load "${@:2}"
      ;;
    switch)
      # IMPORTANT: never call 'trap' inside this function. In zsh, a trap set
      # inside a function fires when the function returns, not when the shell
      # exits. Kubeconfig cleanup is handled by the shell-init EXIT trap below.
      if _kctx_raw="$(command kctx switch "${@:2}")"; then
        eval "$(echo "$_kctx_raw" | grep -v '^trap ')"
        _KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
      fi
      ;;
    unload)
      eval "$(command kctx unload)"
      ;;
    status|clear-cache|watch|shell-init)
      command kctx "$@"
      ;;
    --version|--help|-h)
      command kctx "$@"
      ;;
    "")
      command kctx
      ;;
    *)
      # Positional shorthand: kctx <env> [source] → kctx switch <env> [source]
      if _kctx_raw="$(command kctx switch "$@")"; then
        eval "$(echo "$_kctx_raw" | grep -v '^trap ')"
        _KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
      fi
      ;;
  esac
}

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
  eval "$(command kctx unload 2>/dev/null)" 2>/dev/null || true
}
_KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
if [ -n "${ZSH_VERSION:-}" ]; then
  autoload -Uz add-zsh-hook 2>/dev/null && add-zsh-hook precmd _kctx_check_unload
else
  PROMPT_COMMAND="_kctx_check_unload${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
fi

if [ -z "${_KCTX_WATCH_PID:-}" ]; then
  command kctx watch &
  _KCTX_WATCH_PID=$!
  trap 'command kctx unload >/dev/null 2>&1; kill "${_KCTX_WATCH_PID:-}" 2>/dev/null; command kctx clear-cache 2>/dev/null' EXIT
fi
`
}

func shellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init",
		Short: "Emit kctx shell wrapper function",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(ShellSnippet())
		},
	}
}
