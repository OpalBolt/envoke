package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/eficode/secure-handling-of-secrets/internal/cleanup"
	intkctx "github.com/eficode/secure-handling-of-secrets/internal/cli/kctx"
	intrenv "github.com/eficode/secure-handling-of-secrets/internal/cli/renv"
	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/logger"
	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/eficode/secure-handling-of-secrets/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var verbose bool
	var noCache bool
	var cfgFile string
	var logLevel string
	var cfg config.Config

	root := &cobra.Command{
		Use:   "envoke",
		Short: "Unified secret environment loader — env vars and kubeconfigs",
		Long: `envoke (env + invoke) is a single binary combining renv and kctx.

  envoke resolve .env            # resolve both env secrets and kubeconfig refs
  envoke renv resolve .env       # renv subcommands (env secret resolution)
  envoke kctx switch prod        # kctx subcommands (kubeconfig switching)
  envoke shell-init              # combined shell setup for both renv and kctx

The .env file supports KCTX_<name>=bw://... or KCTX_<name>=vault://...
entries that load kubeconfigs into the local kctx named store.`,
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
			)
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
	root.PersistentFlags().BoolVar(&noCache, "no-cache", false, "Disable encrypted cache")
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	// Embed renv and kctx command trees as subcommands.
	renvRoot := intrenv.NewRootCmd()
	kctxRoot := intkctx.NewRootCmd()

	root.AddCommand(
		renvRoot,
		kctxRoot,
		resolveCmd(&noCache, &cfg),
		shellInitCmd(),
		clearCacheCmd(),
		watchCmd(),
	)
	return root
}

// resolveCmd is the combined resolver: handles both env secrets (bw://, vault://)
// and kubeconfig directives (KCTX_<name>=bw://... or KCTX_<name>=vault://...).
//
// .env format for kubeconfig loading:
//
//	KCTX_PROD=bw://kubernetes/prod-cluster
//	KCTX_STAGING=vault://secret/kubeconfig/staging
//
// The kubeconfig name is derived from the key by stripping the KCTX_ prefix
// and lowercasing: KCTX_PROD → "prod".
func resolveCmd(noCache *bool, cfg *config.Config) *cobra.Command {
	var file string
	var shell string

	cmd := &cobra.Command{
		Use:   "resolve [file]",
		Short: "Resolve .env secrets and kubeconfig directives",
		Long: `Resolve a .env file, handling both secret references and kubeconfig directives.

Secret references (bw://, vault://) are resolved and exported as shell variables.

Kubeconfig directives (keys prefixed with KCTX_) load a kubeconfig into the
local kctx named store without exporting the raw value:

  KCTX_PROD=bw://kubernetes/prod-cluster     # loads kubeconfig named "prod"
  KCTX_STAGING=vault://secret/kubeconfigs    # loads kubeconfig named "staging"

After envoke resolve, switch to a loaded kubeconfig with:

  kctx prod       (shorthand for: kctx switch prod)

The output must be evaluated by your shell:

  eval "$(envoke resolve .env)"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				file = args[0]
			}
			slog.Debug("running envoke resolve", "file", file)
			if term.IsTerminal(int(os.Stdout.Fd())) {
				ui.Warn(os.Stderr, "stdout is a terminal — output will not be set as env vars.")
				fmt.Fprintln(os.Stderr, "  use: eval \"$(envoke resolve .env)\"")
			}

			// Parse the raw .env file to separate kctx directives from env secrets.
			rawEntries, err := env.ParseRaw(file)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", file, err)
			}

			var kctxEntries []env.RawEntry
			var envEntries []env.RawEntry
			for _, e := range rawEntries {
				if isKctxDirective(e) {
					kctxEntries = append(kctxEntries, e)
				} else {
					envEntries = append(envEntries, e)
				}
			}

			// Pre-flight: validate all KCTX names before doing any secret fetching.
			for _, e := range kctxEntries {
				if err := kubeconfig.ValidateStoreName(kctxNameFromKey(e.Key)); err != nil {
					return fmt.Errorf("invalid kctx directive %q: %w", e.Key, err)
				}
			}

			// Create ONE shared cache+clients for the entire resolve operation.
			// This ensures LocalPassword (and BW session) are prompted only once,
			// regardless of how many KCTX_ and bw:// entries the file contains.
			sharedCache := secrets.NewCache()
			sharedCache.MaxAge = cfg.CacheMaxAge()
			if *noCache {
				sharedCache.Disabled = true
			}
			sharedBW := &secrets.BWClient{
				Cache:   sharedCache,
				Timeout: cfg.BitwardenTimeout(),
			}
			sharedVault := &secrets.VaultClient{Timeout: cfg.VaultTimeout()}

			// Handle kubeconfig directives using the shared clients.
			var kctxPanelEntries []ui.PanelEntry
			if len(kctxEntries) > 0 {
				uid := fmt.Sprintf("%d", os.Getuid())
				store := kubeconfig.NewNamedStore()
				for _, e := range kctxEntries {
					name := kctxNameFromKey(e.Key)
					if err := fetchKubeconfigForDirective(sharedBW, sharedVault, name, e.Value, uid, store); err != nil {
						return fmt.Errorf("loading kubeconfig %q (%s): %w", name, e.Value, err)
					}
					kctxPanelEntries = append(kctxPanelEntries, ui.PanelEntry{
						Key:    name,
						Source: e.Value,
					})
					slog.Debug("loaded kubeconfig into kctx store", "name", name, "source", e.Value)
				}
			}

			// Handle env secret entries using the same shared clients.
			// Because sharedBW already has LocalPassword set (from the kctx step above,
			// if any), ensureLocalPassword() returns immediately without re-prompting.
			var resolvedEntries []env.EnvEntry
			if len(envEntries) > 0 {
				tmpFile, err := writeTempEnv(envEntries)
				if err != nil {
					return fmt.Errorf("preparing env entries: %w", err)
				}
				defer os.Remove(tmpFile)

				resolvedEntries, err = env.ResolveDotEnv(tmpFile, sharedBW, sharedVault)
				if err != nil {
					return fmt.Errorf("resolving %s: %w", file, err)
				}
			}

			// Emit shell exports for env entries.
			if err := env.EmitExports(os.Stdout, resolvedEntries); err != nil {
				return err
			}

			// Persist exported key names for envoke renv unload.
			if len(resolvedEntries) > 0 {
				uid := fmt.Sprintf("%d", os.Getuid())
				names := make([]string, len(resolvedEntries))
				for i, e := range resolvedEntries {
					names[i] = e.Key
				}
				_ = secrets.SaveVarNames(uid, names)
			}

			// Feedback to stderr.
			panelEntries := make([]ui.PanelEntry, 0, len(resolvedEntries)+len(kctxPanelEntries))
			for _, e := range resolvedEntries {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: e.Key, Source: e.Source})
			}
			for _, e := range kctxPanelEntries {
				panelEntries = append(panelEntries, ui.PanelEntry{
					Key:    "kctx:" + e.Key,
					Source: e.Source,
				})
			}
			totalCount := len(resolvedEntries) + len(kctxPanelEntries)
			headline := fmt.Sprintf("Loaded %s from %s",
				ui.Bold(os.Stderr, pluralItems(totalCount)),
				ui.Bold(os.Stderr, file))
			ui.Panel(os.Stderr, "envoke", headline, panelEntries, cfg.UI.Border)

			inManagedEnv := os.Getenv("DIRENV_DIR") != "" ||
				os.Getenv("DIRENV_FILE") != "" ||
				os.Getenv("IN_NIX_SHELL") != ""
			if !inManagedEnv {
				switch shell {
				case "fish":
					fmt.Println("# Fish shell trap not supported via eval; use envoke clear-cache manually")
				default:
					fmt.Println("trap 'envoke clear-cache' EXIT")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", ".env", "Path to .env file")
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type (bash|fish|zsh)")
	return cmd
}

// isKctxDirective returns true if the entry is a KCTX_<name>=bw:// or vault:// directive.
func isKctxDirective(e env.RawEntry) bool {
	if !strings.HasPrefix(e.Key, "KCTX_") {
		return false
	}
	return strings.HasPrefix(e.Value, "bw://") || strings.HasPrefix(e.Value, "vault://")
}

// kctxNameFromKey derives a kubeconfig store name from a KCTX_ key.
// KCTX_PROD → "prod", KCTX_MY_CLUSTER → "my_cluster"
func kctxNameFromKey(key string) string {
	return strings.ToLower(strings.TrimPrefix(key, "KCTX_"))
}

// fetchKubeconfigForDirective fetches kubeconfig bytes from a bw:// or vault:// source,
// stores them in the named store, and ensures the shared bwClient has a LocalPassword set.
// Using a shared bwClient means LocalPassword is prompted at most once per process.
func fetchKubeconfigForDirective(bwClient *secrets.BWClient, vaultClient *secrets.VaultClient, name, source, uid string, store *kubeconfig.NamedStore) error {
	var kubeconfigData []byte

	if strings.HasPrefix(source, "bw://") {
		ref, err := secrets.ParseBWRef(source)
		if err != nil {
			return err
		}
		val, err := bwClient.Resolve(ref)
		if err != nil {
			return err
		}
		// bwClient.LocalPassword is now set by Resolve (ensureLocalPassword was called).
		kubeconfigData = []byte(val)
	} else {
		// vault:// path
		var vaultRef secrets.VaultRef
		if strings.HasPrefix(source, "vault://") {
			if strings.Contains(source, "#") {
				ref, err := secrets.ParseVaultRef(source)
				if err != nil {
					return err
				}
				if ref.Field == "" {
					ref.Field = "kubeconfig"
				}
				vaultRef = ref
			} else {
				vaultRef = secrets.VaultRef{
					Path:  strings.TrimPrefix(source, "vault://"),
					Field: "kubeconfig",
				}
			}
		} else {
			vaultRef = secrets.VaultRef{Path: source, Field: "kubeconfig"}
		}
		val, err := vaultClient.Resolve(vaultRef)
		if err != nil {
			return err
		}
		// For vault sources, ensure the shared bwClient has a LocalPassword so store.Put works.
		// If a prior BW operation already set it, this is a no-op.
		if bwClient.LocalPassword == "" {
			lp, err := secrets.ReadLocalPassword()
			if err != nil {
				return err
			}
			bwClient.LocalPassword = lp
		}
		kubeconfigData = []byte(val)
	}

	return store.Put(uid, name, bwClient.LocalPassword, kubeconfigData)
}

// writeTempEnv writes env entries to a temp .env file for processing by ResolveDotEnv.
func writeTempEnv(entries []env.RawEntry) (string, error) {
	f, err := os.CreateTemp("", "envoke-*.env")
	if err != nil {
		return "", err
	}
	defer f.Close()
	for _, e := range entries {
		// Write as key='value' to preserve any special characters.
		if _, err := fmt.Fprintf(f, "%s=%s\n", e.Key, e.Value); err != nil {
			return "", err
		}
	}
	return f.Name(), nil
}

// clearCacheCmd clears both renv and kctx caches.
func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove all envoke cache files (renv + kctx)",
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("clearing all caches", "uid", uid)

			cache := secrets.NewCache()
			if err := cache.Clear(uid); err != nil {
				return fmt.Errorf("clearing secret cache: %w", err)
			}
			if err := secrets.ClearStoredSession(uid); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}
			if err := secrets.ClearStoredLocalPassword(uid); err != nil {
				return fmt.Errorf("clearing local password: %w", err)
			}

			store := kubeconfig.NewNamedStore()
			if err := store.Clear(uid); err != nil {
				return fmt.Errorf("clearing kubeconfig store: %w", err)
			}

			ui.Success(os.Stderr, "All caches cleared")
			return nil
		},
	}
}

// watchCmd runs a combined watcher that handles both renv and kctx cleanup.
func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for sleep/lock events and manage secrets and kubeconfigs",
		Long: `Run in the background to manage secrets and kubeconfigs when the system
sleeps or the screen is locked. Normally started automatically by shell-init.

On lock: secret variables are unloaded and kubeconfig tmpfiles are removed.
On sleep: all caches are cleared, requiring full re-authentication after wake.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			detachFromTerminal()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("starting envoke watcher", "uid", uid)

			hook := cleanup.New()

			if err := hook.RegisterLock(func() error {
				slog.Debug("cleanup: unloading renv variables and kctx kubeconfigs on lock")
				_ = secrets.RequestUnload(uid)
				kubeconfig.ClearManaged()
				_ = kubeconfig.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing all caches on sleep")
				cache := secrets.NewCache()
				_ = cache.Clear(uid)
				_ = secrets.ClearStoredSession(uid)
				_ = secrets.ClearStoredLocalPassword(uid)
				_ = secrets.RequestUnload(uid)
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

// shellInitCmd emits the combined envoke shell-init snippet.
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
      # Auto-eval so secrets and kubeconfigs are loaded into the current shell.
      # Strip the standalone EXIT trap — the shell-init trap below covers cleanup.
      eval "$(command envoke resolve "${@:2}" | grep -v '^trap ')"
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
  trap 'eval "$(command envoke renv unload 2>/dev/null || true)"; command envoke kctx unload >/dev/null 2>&1; kill "${_ENVOKE_WATCH_PID:-}" 2>/dev/null; command envoke clear-cache 2>/dev/null' EXIT
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
      # Auto-eval so secrets and kubeconfigs are loaded into the current shell.
      command envoke resolve $argv[2..] | grep -v '^trap ' | source
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
  if set -q _ENVOKE_WATCH_PID
    kill $_ENVOKE_WATCH_PID 2>/dev/null; or true
    set -e _ENVOKE_WATCH_PID
  end
  command envoke clear-cache 2>/dev/null; or true
end
`

func pluralItems(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
