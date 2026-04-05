package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/eficode/secure-handling-of-secrets/internal/cleanup"
	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/logger"
	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/eficode/secure-handling-of-secrets/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var verbose bool
	var cfgFile string
	var logLevel string
	var cfg config.Config

	root := &cobra.Command{
		Use:     "kctx",
		Short:   "Ephemeral kubeconfig switcher via Vault or Bitwarden",
		Version: version.String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.Load(cfgFile)
			if err != nil {
				return err
			}
			if cmd.Root().PersistentFlags().Changed("log-level") {
				cfg.Log.Level = logLevel
			}
			if verbose {
				cfg.Log.Level = "debug"
			}
			logger.Init(cfg.Log.Level, cfg.Log.Format)
			slog.Debug("config loaded",
				"log_level", cfg.Log.Level,
				"log_format", cfg.Log.Format,
				"timeout_vault", cfg.Timeouts.Vault,
				"timeout_bitwarden", cfg.Timeouts.Bitwarden,
			)
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	root.AddCommand(
		switchCmd(&cfg),
		unloadCmd(),
		statusCmd(),
		clearCacheCmd(),
		shellInitCmd(),
		watchCmd(),
	)
	return root
}

func switchCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <env> [vault-path|bw://item]",
		Short: "Fetch kubeconfig, write to tmpfile, print KUBECONFIG export",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env := args[0]
			source := ""
			if len(args) > 1 {
				source = args[1]
			}
			slog.Debug("switching kubeconfig", "env", env, "source", source)

			var kubeconfigData []byte

			if source == "" || source == env {
				// Default: try vault path based on env name
				vaultRef := secrets.VaultRef{Path: "secret/kubeconfig/" + env, Field: "kubeconfig"}
				vc := &secrets.VaultClient{Timeout: cfg.VaultTimeout()}
				val, verr := vc.Resolve(vaultRef)
				if verr != nil {
					return fmt.Errorf("fetching kubeconfig for %q: %w", env, verr)
				}
				kubeconfigData = []byte(val)
			} else if len(source) > 5 && source[:5] == "bw://" {
				ref, err := secrets.ParseBWRef(source)
				if err != nil {
					return err
				}
				cache := secrets.NewCache()
				cache.MaxAge = cfg.CacheMaxAge()
				bwClient := &secrets.BWClient{
					Cache:   cache,
					Timeout: cfg.BitwardenTimeout(),
				}
				val, bwerr := bwClient.Resolve(ref)
				if bwerr != nil {
					return bwerr
				}
				kubeconfigData = []byte(val)
			} else {
				// Treat as vault path
				vaultRef := secrets.VaultRef{Path: source, Field: "kubeconfig"}
				vc := &secrets.VaultClient{Timeout: cfg.VaultTimeout()}
				val, verr := vc.Resolve(vaultRef)
				if verr != nil {
					return verr
				}
				kubeconfigData = []byte(val)
			}

			path, werr := kubeconfig.WriteKubeconfig(kubeconfigData)
			if werr != nil {
				return fmt.Errorf("writing kubeconfig: %w", werr)
			}

			fmt.Printf("export KUBECONFIG=%s\n", path)
			fmt.Printf("trap 'kctx unload' EXIT\n")

			// Feedback to stderr — stdout must stay clean for eval.
			// Resolve a friendly source label for the panel.
			srcLabel := resolveSourceLabel(env, source)
			panelEntries := []ui.PanelEntry{
				{Key: "KUBECONFIG", Value: path, Source: srcLabel},
			}
			if ctx := currentKubectlContext(path); ctx != "" {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: "Context", Value: ctx})
			}
			headline := fmt.Sprintf("Switched to %s", ui.Bold(os.Stderr, env))
			ui.Panel(os.Stderr, "kctx", headline, panelEntries)
			return nil
		},
	}
}

func unloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Unset KUBECONFIG and remove tmpfile (only if created by kctx)",
		RunE: func(cmd *cobra.Command, args []string) error {
			kubeconfigPath := os.Getenv("KUBECONFIG")
			slog.Debug("clearing kubeconfig", "path", kubeconfigPath)
			if kubeconfigPath != "" && isManagedKubeconfig(kubeconfigPath) {
				_ = os.Remove(kubeconfigPath)
			}
			fmt.Println("unset KUBECONFIG")

			// Feedback to stderr — stdout must stay clean for eval.
			if kubeconfigPath == "" {
				ui.Warn(os.Stderr, "KUBECONFIG was not set")
			} else {
				entries := []ui.PanelEntry{{Key: "Removed", Value: kubeconfigPath}}
				ui.Panel(os.Stderr, "kctx", "Kubeconfig unloaded", entries)
			}
			return nil
		},
	}
}

// isManagedKubeconfig returns true if the path looks like a kctx-created tmpfile.
// Only files under /dev/shm or /tmp with the "kctx-" prefix are considered managed.
func isManagedKubeconfig(path string) bool {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if dir != "/dev/shm" && dir != "/tmp" {
		return false
	}
	return len(base) > 5 && base[:5] == "kctx-"
}

// resolveSourceLabel returns a short human-readable label for where the
// kubeconfig was fetched from, suitable for display in the switch panel.
func resolveSourceLabel(env, source string) string {
	if source == "" || source == env {
		return "vault://secret/kubeconfig/" + env
	}
	return source
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current KUBECONFIG and kubectl context",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := os.Stdout
			ui.Header(w, "Kubeconfig")

			kc := os.Getenv("KUBECONFIG")
			if kc == "" {
				ui.Item(w, "KUBECONFIG", ui.Gray(w, "not set"))
				ui.Item(w, "Managed by kctx", ui.Gray(w, "no"))
			} else {
				ui.Item(w, "KUBECONFIG", ui.Green(w, kc))
				managed := isManagedKubeconfig(kc)
				if managed {
					ui.Item(w, "Managed by kctx", ui.Green(w, "yes"))
				} else {
					ui.Item(w, "Managed by kctx", ui.Yellow(w, "no (external)"))
				}

				// Try to show current kubectl context.
				if ctx := currentKubectlContext(kc); ctx != "" {
					ui.Item(w, "Current context", ui.Bold(w, ctx))
				}
			}
			return nil
		},
	}
}

// currentKubectlContext runs kubectl config current-context and returns the
// result, or "" if kubectl is not available or the call fails.
// If kubeconfig is non-empty it is used as the KUBECONFIG path for the invocation.
func currentKubectlContext(kubeconfig string) string {
	cmd := exec.Command("kubectl", "config", "current-context")
	if kubeconfig != "" {
		env := make([]string, 0, len(os.Environ())-1)
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "KUBECONFIG=") {
				env = append(env, e)
			}
		}
		cmd.Env = append(env, "KUBECONFIG="+kubeconfig)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove all kctx cache files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := secrets.NewCache()
			uid := fmt.Sprintf("%d", os.Getuid())
			if err := cache.Clear(uid); err != nil {
				return err
			}
			ui.Success(os.Stderr, "Cache cleared")
			return nil
		},
	}
}

func shellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init",
		Short: "Emit kctx shell wrapper function",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(kctxShellSnippet())
		},
	}
}

func kctxShellSnippet() string {
	return `
# kctx shell integration — add to ~/.bashrc or ~/.zshrc:
# eval "$(kctx shell-init)"

kctx() {
  case "$1" in
    switch)
      # Explicit subcommand: kctx switch <env> [source] [flags]
      # Only eval and arm the EXIT trap when switch succeeds; a failing switch
      # must not replace the shell-init EXIT trap or unload a working kubeconfig.
      if _kctx_out="$(command kctx switch "${@:2}")"; then
        eval "$_kctx_out"
        # Record current sentinel token so a stale pre-existing sentinel
        # doesn't trigger immediate unload on the next prompt.
        _KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
        trap 'kctx unload; kill "${_KCTX_WATCH_PID:-}" 2>/dev/null; command kctx clear-cache 2>/dev/null' EXIT
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
      # No arguments: show root-level help (list all subcommands).
      command kctx
      ;;
    *)
      # Positional shorthand: kctx <env> [source] → kctx switch <env> [source]
      if _kctx_out="$(command kctx switch "$@")"; then
        eval "$_kctx_out"
        _KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
        trap 'kctx unload; kill "${_KCTX_WATCH_PID:-}" 2>/dev/null; command kctx clear-cache 2>/dev/null' EXIT
      fi
      ;;
  esac
}

# Derive an idempotent token for the current unload request so each shell can
# observe the event once without consuming the shared sentinel.
_kctx_unload_token() {
  local f="/dev/shm/kctx-${UID}-unload-requested"
  [ -f "$f" ] || f="/tmp/kctx-${UID}-unload-requested"
  [ -f "$f" ] || return 1
  # -c is GNU/Linux (coreutils); -f is BSD/macOS (stat(1)).
  stat -c '%Y:%i:%s' "$f" 2>/dev/null || stat -f '%m:%i:%z' "$f" 2>/dev/null
}

# Unset KUBECONFIG when the watcher signals that sleep/lock occurred.
# Each shell tracks the last token it acted on so only the first prompt after
# the event triggers unload — subsequent prompts are no-ops until the next event.
_kctx_check_unload() {
  local token
  token="$(_kctx_unload_token)" || return 0
  [ "${_KCTX_LAST_UNLOAD_TOKEN:-}" = "$token" ] && return 0
  _KCTX_LAST_UNLOAD_TOKEN="$token"
  eval "$(command kctx unload 2>/dev/null)" 2>/dev/null || true
}
# Record the current sentinel state at init time so pre-existing sentinels from
# a previous session do not trigger an immediate unload in new shells.
_KCTX_LAST_UNLOAD_TOKEN="$(_kctx_unload_token 2>/dev/null || true)"
if [ -n "${ZSH_VERSION:-}" ]; then
  autoload -Uz add-zsh-hook 2>/dev/null && add-zsh-hook precmd _kctx_check_unload
else
  PROMPT_COMMAND="_kctx_check_unload${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
fi

# Start the sleep/lock watcher once per shell session.
if [ -z "${_KCTX_WATCH_PID:-}" ]; then
  command kctx watch &
  _KCTX_WATCH_PID=$!
  trap 'kill "${_KCTX_WATCH_PID:-}" 2>/dev/null; command kctx clear-cache 2>/dev/null' EXIT
fi
`
}

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for sleep/lock events and manage kubeconfigs (run in background by shell-init)",
		Long: `Run in the background to manage kubeconfig tmpfiles and the secret cache
when the system sleeps or the screen is locked. Normally started automatically
by shell-init.

On lock: managed kubeconfig tmpfiles are removed and open shells are signalled
to unset KUBECONFIG. The encrypted cache is kept so configs can be quickly
re-resolved after unlock without re-entering passwords.

On sleep: the encrypted cache is also cleared, requiring full re-authentication
after wake.

On Linux, sleep and screen-lock events are detected via D-Bus (systemd-logind).
On macOS, sleep is detected via timer drift; screen lock requires a launchd agent.
On Windows, event hooks are not yet implemented.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			detachFromTerminal()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("starting kctx watcher", "uid", uid)

			hook := cleanup.New()

			// On lock: remove managed kubeconfig tmpfiles and unload KUBECONFIG.
			// The cache is kept so configs can be re-resolved after unlock
			// without re-entering passwords.
			if err := hook.RegisterLock(func() error {
				slog.Debug("cleanup: clearing managed kubeconfigs on lock")
				kubeconfig.ClearManaged()
				_ = kubeconfig.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			// On sleep: clear the encrypted cache as well.
			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing kctx cache and managed kubeconfigs on sleep")
				cache := secrets.NewCache()
				_ = cache.Clear(uid)
				kubeconfig.ClearManaged()
				_ = kubeconfig.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering sleep hook: %w", err)
			}

			defer hook.Unregister()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
			<-sigCh
			return nil
		},
	}
}
