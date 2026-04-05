package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
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
		loadCmd(&cfg),
		switchCmd(&cfg),
		unloadCmd(),
		statusCmd(),
		clearCacheCmd(),
		shellInitCmd(),
		watchCmd(),
	)
	return root
}

// loadCmd fetches a kubeconfig from Bitwarden or Vault and stores it in the
// named store, encrypted with the local password. Designed to be called from
// a .env file so multiple configs can be pre-loaded before switching.
//
// Usage:
//
//	kctx load prod bw://folder/item
//	kctx load staging vault://secret/kubeconfig/staging
func loadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "load <name> <bw://item|vault-path>",
		Short: "Fetch a kubeconfig and cache it under a local name",
		Long: `Fetch a kubeconfig from Bitwarden or Vault and encrypt it in the local
named store so that 'kctx switch <name>' can load it without re-fetching.

Place multiple kctx load calls in your .env file to pre-load all configs.
Both the local password and the Bitwarden password are prompted fresh on
every call — no passwords are persisted or shared between invocations.

Examples:
  kctx load prod bw://kubernetes/prod
  kctx load staging vault://secret/kubeconfig/staging`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			source := args[1]

			if err := kubeconfig.ValidateStoreName(name); err != nil {
				return err
			}

			slog.Debug("loading kubeconfig", "name", name, "source", source)

			uid := fmt.Sprintf("%d", os.Getuid())
			store := kubeconfig.NewNamedStore()

			var kubeconfigData []byte
			var localPassword string

			if strings.HasPrefix(source, "bw://") {
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
				localPassword = bwClient.LocalPassword
			} else {
				// Parse vault:// URI (with optional #field fragment) or treat as raw path.
				var vaultRef secrets.VaultRef
				if strings.HasPrefix(source, "vault://") {
					if strings.Contains(source, "#") {
						// Fragment present — parse it fully.
						ref, err := secrets.ParseVaultRef(source)
						if err != nil {
							return err
						}
						if ref.Field == "" {
							ref.Field = "kubeconfig"
						}
						vaultRef = ref
					} else {
						// No fragment — default field to "kubeconfig".
						vaultRef = secrets.VaultRef{
							Path:  strings.TrimPrefix(source, "vault://"),
							Field: "kubeconfig",
						}
					}
				} else {
					vaultRef = secrets.VaultRef{Path: source, Field: "kubeconfig"}
				}
				vc := &secrets.VaultClient{Timeout: cfg.VaultTimeout()}
				val, verr := vc.Resolve(vaultRef)
				if verr != nil {
					return verr
				}
				kubeconfigData = []byte(val)
				// Vault doesn't use local password; prompt separately.
				lp, lperr := secrets.ReadLocalPassword()
				if lperr != nil {
					return lperr
				}
				localPassword = lp
			}

			if err := store.Put(uid, name, localPassword, kubeconfigData); err != nil {
				return fmt.Errorf("storing kubeconfig %q: %w", name, err)
			}

			ui.Success(os.Stderr, fmt.Sprintf("Loaded kubeconfig: %s", ui.Bold(os.Stderr, name)))
			return nil
		},
	}
}

// switchCmd loads a pre-cached (named) kubeconfig and exports KUBECONFIG.
// If the name is not found in the local named store, it falls back to an
// explicit bw:// or vault path source if one is provided.
func switchCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name> [bw://item|vault-path]",
		Short: "Switch to a named kubeconfig (or fetch one if a source is given)",
		Long: `Switch KUBECONFIG to a named kubeconfig previously loaded with 'kctx load'.

If the named kubeconfig is not in the local store, a source (bw:// or vault
path) may be provided to fetch it on the fly.

Examples:
  kctx switch prod                          # use pre-loaded 'prod'
  kctx switch staging bw://k8s/staging      # fetch directly if not pre-loaded`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			source := ""
			if len(args) > 1 {
				source = args[1]
			}
			slog.Debug("switching kubeconfig", "name", name, "source", source)

			uid := fmt.Sprintf("%d", os.Getuid())
			store := kubeconfig.NewNamedStore()

			var kubeconfigData []byte

			// Try the named store first.
			if err := kubeconfig.ValidateStoreName(name); err == nil {
				lp, lperr := secrets.ReadLocalPassword()
				if lperr != nil {
					return lperr
				}
				data, err := store.Get(uid, name, lp)
				if err != nil {
					return fmt.Errorf("reading named kubeconfig %q: %w", name, err)
				}
				if data != nil {
					kubeconfigData = data
				}
			}

			// Fall back to an explicit source if the named store had no entry.
			if kubeconfigData == nil {
				if source == "" {
					return fmt.Errorf(
						"no pre-loaded kubeconfig named %q found\n"+
							"Run: kctx load %s <bw://item|vault-path>",
						name, name,
					)
				}
				var err error
				kubeconfigData, err = fetchKubeconfig(cfg, source)
				if err != nil {
					return fmt.Errorf("fetching kubeconfig for %q: %w", name, err)
				}
			}

			path, werr := kubeconfig.WriteKubeconfig(kubeconfigData)
			if werr != nil {
				return fmt.Errorf("writing kubeconfig: %w", werr)
			}

			fmt.Printf("export KUBECONFIG=%s\n", path)
			fmt.Printf("trap 'kctx unload' EXIT\n")

			// Feedback to stderr — stdout must stay clean for eval.
			// Resolve a friendly source label for the panel.
			srcLabel := resolveSourceLabel(name, source)
			panelEntries := []ui.PanelEntry{
				{Key: "KUBECONFIG", Value: path, Source: srcLabel},
			}
			if ctx := currentKubectlContext(path); ctx != "" {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: "Context", Value: ctx})
			}
			headline := fmt.Sprintf("Switched to %s", ui.Bold(os.Stderr, name))
			ui.Panel(os.Stderr, "kctx", headline, panelEntries)
			return nil
		},
	}
}

// fetchKubeconfig fetches kubeconfig bytes from the given bw:// or vault source.
func fetchKubeconfig(cfg *config.Config, source string) ([]byte, error) {
	if strings.HasPrefix(source, "bw://") {
		ref, err := secrets.ParseBWRef(source)
		if err != nil {
			return nil, err
		}
		cache := secrets.NewCache()
		cache.MaxAge = cfg.CacheMaxAge()
		bwClient := &secrets.BWClient{
			Cache:   cache,
			Timeout: cfg.BitwardenTimeout(),
		}
		val, bwerr := bwClient.Resolve(ref)
		if bwerr != nil {
			return nil, bwerr
		}
		return []byte(val), nil
	}
	// Parse vault:// URI (with optional #field fragment) or treat as raw path.
	var vaultRef secrets.VaultRef
	if strings.HasPrefix(source, "vault://") {
		if strings.Contains(source, "#") {
			// Fragment present — parse it fully.
			ref, err := secrets.ParseVaultRef(source)
			if err != nil {
				return nil, err
			}
			if ref.Field == "" {
				ref.Field = "kubeconfig"
			}
			vaultRef = ref
		} else {
			// No fragment — default field to "kubeconfig".
			vaultRef = secrets.VaultRef{
				Path:  strings.TrimPrefix(source, "vault://"),
				Field: "kubeconfig",
			}
		}
	} else {
		vaultRef = secrets.VaultRef{Path: source, Field: "kubeconfig"}
	}
	vc := &secrets.VaultClient{Timeout: cfg.VaultTimeout()}
	val, verr := vc.Resolve(vaultRef)
	if verr != nil {
		return nil, verr
	}
	return []byte(val), nil
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
func resolveSourceLabel(name, source string) string {
	if source == "" {
		return "named store"
	}
	return source
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current KUBECONFIG, kubectl context, and loaded kubeconfigs",
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

			// Show named kubeconfigs available in the local store.
			uid := fmt.Sprintf("%d", os.Getuid())
			store := kubeconfig.NewNamedStore()
			names, err := store.List(uid)
			if err != nil {
				slog.Warn("listing named kubeconfigs", "err", err)
			} else if len(names) > 0 {
				sort.Strings(names)
				ui.Header(w, "Loaded kubeconfigs (use 'kctx switch <name>')")
				ui.List(w, names)
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
		Short: "Remove all kctx cache files and named kubeconfigs",
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())

			cache := secrets.NewCache()
			if err := cache.Clear(uid); err != nil {
				return err
			}

			store := kubeconfig.NewNamedStore()
			if err := store.Clear(uid); err != nil {
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
    load)
      # Pre-load a named kubeconfig from Bitwarden or Vault.
      # Passwords are prompted fresh on every call — no caching or persistence.
      #
      # Usage in .env:
      #   kctx load prod     bw://kubernetes/prod
      #   kctx load staging  bw://kubernetes/staging
      command kctx load "${@:2}"
      ;;
    switch)
      # Explicit subcommand: kctx switch <name> [source] [flags]
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
      # Positional shorthand: kctx <name> [source] → kctx switch <name> [source]
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

			// On sleep: clear the encrypted cache and named kubeconfig store as well.
			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing kctx cache and managed kubeconfigs on sleep")
				cache := secrets.NewCache()
				_ = cache.Clear(uid)
				store := kubeconfig.NewNamedStore()
				_ = store.Clear(uid)
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
