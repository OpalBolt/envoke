package kctx

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/opalbolt/envoke/internal/cleanup"
	"github.com/opalbolt/envoke/internal/config"
	"github.com/opalbolt/envoke/internal/kubeconfig"
	"github.com/opalbolt/envoke/internal/logger"
	"github.com/opalbolt/envoke/internal/providers"
	bw "github.com/opalbolt/envoke/internal/providers/bitwarden"
	vlt "github.com/opalbolt/envoke/internal/providers/vault"
	"github.com/opalbolt/envoke/internal/securedir"
	"github.com/opalbolt/envoke/internal/ui"
	"github.com/opalbolt/envoke/internal/version"
)

// NewRootCmd returns the root cobra command for the kctx CLI.
func NewRootCmd() *cobra.Command {
	var verbose bool
	var cfgFile string
	var logLevel string
	var cfg config.Config

	root := &cobra.Command{
		Use:          "kctx",
		Short:        "Ephemeral kubeconfig switcher via Vault or Bitwarden",
		Version:      version.String(),
		SilenceUsage: true,
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
		unloadCmd(&cfg),
		statusCmd(),
		shellInitCmd(),
		watchCmd(),
	)
	return root
}

// NewSubCmd returns the kctx subcommand tree for embedding under another root (e.g. envoke).
// Unlike NewRootCmd, it does not register persistent flags (those are inherited from the parent)
// and does not install its own PersistentPreRunE (the parent's runs instead).
// cfg is a pointer owned by the parent, populated before any subcommand runs.
func NewSubCmd(cfg *config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "kctx",
		Short: "Ephemeral kubeconfig switcher via Vault or Bitwarden",
	}
	root.AddCommand(
		loadCmd(cfg),
		switchCmd(cfg),
		unloadCmd(cfg),
		statusCmd(),
		shellInitCmd(),
		watchCmd(),
	)
	return root
}

// ── registry ──────────────────────────────────────────────────────────────────

func newRegistry(cfg *config.Config) *providers.Registry {
	bwClient := &bw.BWClient{
		Timeout: cfg.BitwardenTimeout(),
	}
	vaultClient := &vlt.VaultClient{Timeout: cfg.VaultTimeout()}

	reg := providers.NewRegistry()
	reg.Register(providers.NewBWProvider(bwClient))
	reg.Register(providers.NewVaultProvider(vaultClient))
	return reg
}

// normalizeKubeconfigURI delegates to kubeconfig.NormalizeSourceURI.
func normalizeKubeconfigURI(source string) string {
	return kubeconfig.NormalizeSourceURI(source)
}

// ── load ──────────────────────────────────────────────────────────────────────

func loadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "load <name> <bw://item|vault-path>",
		Short: "Fetch a kubeconfig and cache it under a local name",
		Long: `Fetch a kubeconfig from Bitwarden or Vault and store it in the local
named store so that 'kctx switch <name>' can load it without re-fetching.

Place multiple kctx load calls in your .env file to pre-load all configs.
The Bitwarden password is prompted fresh on every call — no passwords are
persisted or shared between invocations.

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

			reg := newRegistry(cfg)
			uri := normalizeKubeconfigURI(source)

			val, err := reg.Resolve(uri)
			if err != nil {
				return err
			}
			kubeconfigData := []byte(val)

			if err := store.Put(uid, name, kubeconfigData); err != nil {
				return fmt.Errorf("storing kubeconfig %q: %w", name, err)
			}

			ui.Success(os.Stderr, fmt.Sprintf("Loaded kubeconfig: %s", ui.Bold(os.Stderr, name)))
			return nil
		},
	}
}

// ── switch ────────────────────────────────────────────────────────────────────

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

			if err := kubeconfig.ValidateStoreName(name); err == nil {
				data, err := store.Get(uid, name)
				if err != nil {
					return fmt.Errorf("reading named kubeconfig %q: %w", name, err)
				}
				if data != nil {
					kubeconfigData = data
				}
			}

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
				if err := store.Put(uid, name, kubeconfigData); err != nil {
					return fmt.Errorf("caching kubeconfig %q: %w", name, err)
				}
			}

			path := store.Path(uid, name)

			fmt.Printf("export KUBECONFIG=%s\n", path)
			fmt.Printf("trap 'kctx unload' EXIT\n")

			srcLabel := resolveSourceLabel(name, source)
			panelEntries := []ui.PanelEntry{
				{Key: "KUBECONFIG", Value: path, Source: srcLabel},
			}
			if ctx := currentKubectlContext(path); ctx != "" {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: "Context", Value: ctx})
			}
			headline := fmt.Sprintf("Switched to %s", ui.Bold(os.Stderr, name))
			ui.Panel(os.Stderr, "kctx", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
}

func fetchKubeconfig(cfg *config.Config, source string) ([]byte, error) {
	reg := newRegistry(cfg)
	uri := normalizeKubeconfigURI(source)
	val, err := reg.Resolve(uri)
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func resolveSourceLabel(name, source string) string {
	if source == "" {
		return "named store"
	}
	return source
}

// ── shell-init ────────────────────────────────────────────────────────────────

// ShellSnippet returns the kctx bash/zsh shell init snippet.
// The sentinel file directory is resolved at emit time via securedir.Dir() so
// the shell does not need to probe multiple candidate paths.
// Exported so envoke can reference it when building the combined shell-init.
func ShellSnippet() string {
	secDir := securedir.Dir()
	const tmpl = `
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
    status|watch|shell-init)
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
  local f="{{SECUREDIR}}/kctx-${UID}-unload-requested"
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
  trap 'command kctx unload >/dev/null 2>&1; kill "${_KCTX_WATCH_PID:-}" 2>/dev/null' EXIT
fi
`
	return strings.Replace(tmpl, "{{SECUREDIR}}", secDir, 1)
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

// ── status ────────────────────────────────────────────────────────────────────

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

				if ctx := currentKubectlContext(kc); ctx != "" {
					ui.Item(w, "Current context", ui.Bold(w, ctx))
				}
			}

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

func currentKubectlContext(kubeconfigPath string) string {
	cmd := exec.Command("kubectl", "config", "current-context")
	if kubeconfigPath != "" {
		env := make([]string, 0, len(os.Environ())-1)
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "KUBECONFIG=") {
				env = append(env, e)
			}
		}
		cmd.Env = append(env, "KUBECONFIG="+kubeconfigPath)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ── unload ────────────────────────────────────────────────────────────────────

func unloadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Unset KUBECONFIG and clear all cached kubeconfigs",
		RunE: func(cmd *cobra.Command, args []string) error {
			kubeconfigPath := os.Getenv("KUBECONFIG")
			slog.Debug("clearing kubeconfig", "path", kubeconfigPath)
			// Remove tmpfiles if present (legacy or external callers).
			if kubeconfig.IsManagedTemp(kubeconfigPath) {
				_ = os.Remove(kubeconfigPath)
			}
			fmt.Println("unset KUBECONFIG")

			// Clear all named store kubeconfigs and any remaining tmpfiles.
			uid := fmt.Sprintf("%d", os.Getuid())
			store := kubeconfig.NewNamedStore()
			_ = store.Clear(uid)
			kubeconfig.ClearManaged()

			if kubeconfigPath == "" {
				ui.Warn(os.Stderr, "KUBECONFIG was not set")
			} else {
				entries := []ui.PanelEntry{{Key: "KUBECONFIG", Value: kubeconfigPath}}
				ui.Panel(os.Stderr, "kctx", "Kubeconfig unloaded", entries, cfg.UI.Border)
			}
			return nil
		},
	}
}

func isManagedKubeconfig(path string) bool {
	return kubeconfig.IsManaged(path)
}

// ── watch ─────────────────────────────────────────────────────────────────────

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for sleep/lock events and manage kubeconfigs (run in background by shell-init)",
		Long: `Run in the background to manage kubeconfig tmpfiles when the system sleeps
or the screen is locked. Normally started automatically by shell-init.

On lock: managed kubeconfig tmpfiles are removed and open shells are signalled
to unset KUBECONFIG.

On sleep: managed kubeconfig tmpfiles and named kubeconfigs are cleared.

On Linux, sleep and screen-lock events are detected via D-Bus (systemd-logind).
On macOS, sleep is detected via timer drift; screen lock requires a launchd agent.
On Windows, event hooks are not yet implemented.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			detachFromTerminal()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("starting kctx watcher", "uid", uid)

			hook := cleanup.New()

			if err := hook.RegisterLock(func() error {
				slog.Debug("cleanup: clearing managed kubeconfigs on lock")
				kubeconfig.ClearManaged()
				_ = kubeconfig.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing named and managed kubeconfigs on sleep")
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
