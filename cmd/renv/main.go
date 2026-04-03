package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/eficode/secure-handling-of-secrets/internal/cleanup"
	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/env"
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
	var isolated bool
	var passwordGracePeriod string
	var cfgFile string
	var logLevel string
	var cfg config.Config

	root := &cobra.Command{
		Use:     "renv",
		Short:   "Resolve secret references in .env and YAML files",
		Version: version.String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.Load(cfgFile)
			if err != nil {
				return err
			}
			// CLI flags override config/env
			if cmd.Flags().Changed("log-level") || cmd.Root().PersistentFlags().Changed("log-level") {
				cfg.Log.Level = logLevel
			}
			if verbose {
				cfg.Log.Level = "debug"
			}
			if cmd.Root().PersistentFlags().Changed("isolated") {
				cfg.Cache.Isolated = isolated
			}
			if cmd.Root().PersistentFlags().Changed("password-grace-period") {
				cfg.Cache.PasswordGracePeriod = passwordGracePeriod
			}
			logger.Init(cfg.Log.Level, cfg.Log.Format)
			slog.Debug("config loaded",
				"log_level", cfg.Log.Level,
				"log_format", cfg.Log.Format,
				"cache_max_age", cfg.Cache.MaxAge,
				"cache_isolated", cfg.Cache.Isolated,
				"cache_password_grace_period", cfg.Cache.PasswordGracePeriod,
				"timeout_bitwarden", cfg.Timeouts.Bitwarden,
				"timeout_vault", cfg.Timeouts.Vault,
			)
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
	root.PersistentFlags().BoolVar(&noCache, "no-cache", false, "Disable encrypted cache")
	root.PersistentFlags().BoolVar(&isolated, "isolated", false, "Require local password in each terminal (disable cross-terminal sharing)")
	root.PersistentFlags().StringVar(&passwordGracePeriod, "password-grace-period", "", "Grace period before re-prompting for local password (e.g. 1m, 5m, 1h). When set, each terminal authenticates independently; re-prompt is skipped within the period.")
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	root.AddCommand(
		resolveCmd(&noCache, &cfg),
		execCmd(&noCache, &cfg),
		shellInitCmd(),
		yamlCmd(&cfg),
		clearCacheCmd(),
		statusCmd(),
		unloadCmd(),
		watchCmd(),
	)
	return root
}

func newClients(noCache bool, cfg *config.Config) (*secrets.Cache, *secrets.BWClient, *secrets.VaultClient) {
	cache := secrets.NewCache()
	cache.MaxAge = cfg.CacheMaxAge()
	if noCache {
		cache.Disabled = true
	}
	bwClient := &secrets.BWClient{
		Cache:               cache,
		Timeout:             cfg.BitwardenTimeout(),
		Isolated:            cfg.Cache.Isolated,
		PasswordGracePeriod: cfg.CachePasswordGracePeriod(),
	}
	vaultClient := &secrets.VaultClient{Timeout: cfg.VaultTimeout()}
	return cache, bwClient, vaultClient
}

func resolveCmd(noCache *bool, cfg *config.Config) *cobra.Command {
	var file string
	var shell string

	cmd := &cobra.Command{
		Use:   "resolve [file]",
		Short: "Resolve .env file secret references and emit shell exports",
		Long: `Resolve secret references in a .env file and print shell export statements.

The output must be evaluated by your shell to set the variables:

  eval "$(renv resolve .env)"

With direnv, use a use_renv helper so direnv fully owns the load/unload lifecycle.
Add to ~/.config/direnv/direnvrc:

  use_renv() {
    local file="${1:-.env}"
    watch_file "$file"
    eval "$(renv unload 2>/dev/null || true)"
    eval "$(renv resolve "$file")"
  }

Then in .envrc:

  use renv .env`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				file = args[0]
			}
			slog.Debug("running resolve", "file", file, "shell", shell)
			if term.IsTerminal(int(os.Stdout.Fd())) {
				ui.Warn(os.Stderr, "stdout is a terminal — output will not be set as env vars.")
				fmt.Fprintln(os.Stderr, "  use: eval \"$(renv resolve .env)\"")
			}
			_, bwClient, vaultClient := newClients(*noCache, cfg)

			entries, err := env.ResolveDotEnv(file, bwClient, vaultClient)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", file, err)
			}

			if err := env.EmitExports(os.Stdout, entries); err != nil {
				return err
			}

			// Persist the exported key names so renv unload can emit the correct unset commands.
			uid := fmt.Sprintf("%d", os.Getuid())
			names := make([]string, len(entries))
			for i, e := range entries {
				names[i] = e.Key
			}
			_ = secrets.SaveVarNames(uid, names) // best-effort; don't fail resolve if state can't be saved

			// Feedback to stderr — stdout must stay clean for eval.
			ui.Success(os.Stderr, fmt.Sprintf("Loaded %s from %s",
				ui.Bold(os.Stderr, pluralVars(len(entries))),
				ui.Bold(os.Stderr, file)))
			ui.List(os.Stderr, names)

			// Emit EXIT trap — skip inside direnv (and inside nix dev-shells spawned by
			// direnv's use_flake) because the process exits immediately after .envrc is
			// evaluated, which would fire the trap and clear the cache before the user
			// ever gets to use the loaded variables.
			inManagedEnv := os.Getenv("DIRENV_DIR") != "" ||
				os.Getenv("DIRENV_FILE") != "" ||
				os.Getenv("IN_NIX_SHELL") != ""
			if !inManagedEnv {
				switch shell {
				case "fish":
					fmt.Println("# Fish shell trap not supported via eval; use renv clear-cache manually")
				default:
					fmt.Println("trap 'renv clear-cache' EXIT")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", ".env", "Path to .env file")
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type (bash|fish|zsh)")
	return cmd
}

func execCmd(noCache *bool, cfg *config.Config) *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "exec -- command [args...]",
		Short: "Run a command with resolved env vars injected (no eval needed)",
		Long: `Resolve secret references from a .env file and execute a command with those
variables set in its environment. No eval required.

  renv exec -- myprogram --flag value
  renv exec --env secrets.env -- myprogram

The -- separator is required to distinguish renv flags from the command's args.
The resolved variables override any same-named variables already in the environment.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.Debug("running exec", "file", file, "command", args[0])
			_, bwClient, vaultClient := newClients(*noCache, cfg)

			entries, err := env.ResolveDotEnv(file, bwClient, vaultClient)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", file, err)
			}

			// Build env: start with current environment, then append resolved vars.
			// Appending after means the resolved vars win on conflict.
			environ := os.Environ()
			for _, e := range entries {
				environ = append(environ, e.Key+"="+e.Value)
			}

			bin, err := exec.LookPath(args[0])
			if err != nil {
				return fmt.Errorf("%s: command not found", args[0])
			}

			// Replace the current process with the target command.
			return syscall.Exec(bin, args, environ)
		},
	}
	cmd.Flags().StringVarP(&file, "env", "e", ".env", "Path to .env file")
	return cmd
}

// bashInitScript is the shell function emitted by `renv shell-init` for bash/zsh.
// It wraps `resolve` and `unload` with eval so the user never has to type it.
// It also starts a background watcher that clears the cache on sleep/lock.
const bashInitScript = `renv() {
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
# Reading metadata instead of consuming the file lets every open shell
# observe the same event once without racing to delete it.
_renv_unload_token() {
  local f="/dev/shm/renv-${UID}-unload-requested"
  [ -f "$f" ] || f="/tmp/renv-${UID}-unload-requested"
  [ -f "$f" ] || return 1
  # -c is GNU/Linux (coreutils); -f is BSD/macOS (stat(1)).
  stat -c '%Y:%i:%s' "$f" 2>/dev/null || stat -f '%m:%i:%z' "$f" 2>/dev/null
}

# Unload secret variables when the watcher signals that sleep/lock occurred.
# Each shell tracks the last token it acted on so only the first prompt after
# the event triggers unload — subsequent prompts are no-ops until the next event.
_renv_check_unload() {
  local token
  token="$(_renv_unload_token)" || return 0
  [ "${_RENV_LAST_UNLOAD_TOKEN:-}" = "$token" ] && return 0
  _RENV_LAST_UNLOAD_TOKEN="$token"
  eval "$(command renv unload 2>/dev/null)" 2>/dev/null || true
}
# Record the current sentinel state at init time so pre-existing sentinels from
# a previous session do not trigger an immediate unload in new shells.
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

// fishInitScript is the shell function emitted by `renv shell-init --shell fish`.
const fishInitScript = `function renv
  switch $argv[1]
    case resolve unload
      command renv $argv | source
    case '*'
      command renv $argv
  end
end

# Return a token for the current unload sentinel (mtime:inode:size).
# Reading metadata instead of consuming the file lets every open shell
# observe the same event once without racing to delete it.
function _renv_unload_token
  set -l f /dev/shm/renv-(id -u)-unload-requested
  test -f $f; or set f /tmp/renv-(id -u)-unload-requested
  test -f $f; or return 1
  # -c is GNU/Linux (coreutils); -f is BSD/macOS (stat(1)).
  stat -c '%Y:%i:%s' $f 2>/dev/null; or stat -f '%m:%i:%z' $f 2>/dev/null
end

# Unload secret variables when the watcher signals that sleep/lock occurred.
# Each shell tracks the last token it acted on so only the first prompt after
# the event triggers unload — subsequent prompts are no-ops until the next event.
function _renv_check_unload --on-event fish_prompt
  set -l token (_renv_unload_token 2>/dev/null); or return
  test "$_RENV_LAST_UNLOAD_TOKEN" = "$token"; and return
  set -g _RENV_LAST_UNLOAD_TOKEN $token
  command renv unload | source 2>/dev/null; or true
end
# Record the current sentinel state at init time so pre-existing sentinels
# do not trigger an immediate unload in new shells.
set -g _RENV_LAST_UNLOAD_TOKEN (_renv_unload_token 2>/dev/null; or echo "")

# Start the sleep/lock watcher once per shell session.
if not set -q _RENV_WATCH_PID
  command renv watch &
  set -gx _RENV_WATCH_PID $last_pid
end

# Terminate the watcher and clear sensitive cache/session material on shell exit.
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
				_, err := io.WriteString(cmd.OutOrStdout(), fishInitScript)
				return err
			default:
				_, err := io.WriteString(cmd.OutOrStdout(), bashInitScript)
				return err
			}
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type: bash, zsh, fish")
	return cmd
}

func yamlCmd(cfg *config.Config) *cobra.Command {
	var file string
	var key string

	cmd := &cobra.Command{
		Use:   "yaml [file]",
		Short: "Resolve secret references in a YAML file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				file = args[0]
			}
			if file == "" {
				return fmt.Errorf("--file or positional argument required")
			}
			slog.Debug("running yaml resolve", "file", file, "key", key)
			_, bwClient, vaultClient := newClients(false, cfg)

			data, err := env.ResolveYAML(file, bwClient, vaultClient)
			if err != nil {
				return err
			}

			if key != "" {
				val, err := env.YAMLLookup(data, key)
				if err != nil {
					return err
				}
				fmt.Println(val)
				return nil
			}

			out, err := env.MarshalYAML(data)
			if err != nil {
				return err
			}
			fmt.Print(string(out))
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to YAML file")
	cmd.Flags().StringVar(&key, "key", "", "Dot-notation key to extract (e.g. database.password)")
	return cmd
}

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove all renv cache files and stored session",
		Long: `Remove the encrypted secret cache and stored Bitwarden session.

Variable name tracking (used by renv unload) is intentionally preserved so that
renv unload continues to work after a cache clear — for example when the EXIT
trap fires inside a direnv subprocess.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := secrets.NewCache()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("clearing cache and session", "uid", uid)
			if err := cache.Clear(uid); err != nil {
				return fmt.Errorf("clearing cache: %w", err)
			}
			if err := secrets.ClearStoredSession(uid); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}
			if err := secrets.ClearStoredLocalPassword(uid); err != nil {
				return fmt.Errorf("clearing local password: %w", err)
			}
			// Var-name tracking is not cleared here — that is renv unload's job.
			// Keeping the names file intact ensures renv unload remains functional
			// even when clear-cache is triggered by the shell EXIT trap.
			ui.Success(os.Stderr, "Cache cleared")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cache and loaded variable status",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := os.Stdout
			uid := fmt.Sprintf("%d", os.Getuid())

			// ── Tracked variables ────────────────────────────────────
			ui.Header(w, "Tracked variables")
			names, err := secrets.LoadVarNames(uid)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				ui.Item(w, "Status", ui.Gray(w, "none loaded"))
			} else {
				ui.Item(w, "Status", ui.Green(w, fmt.Sprintf("%d loaded", len(names))))
				ui.List(w, names)
			}
			fmt.Fprintln(w)

			// ── Cache files ──────────────────────────────────────────
			ui.Header(w, "Cache")
			cache := secrets.NewCache()
			ui.Item(w, "Location", cache.Dir)
			files, ages, err := secrets.CacheStatus(cache)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				ui.Item(w, "Files", ui.Gray(w, "none"))
			} else {
				for i, f := range files {
					age, err := time.ParseDuration(ages[i])
					ageStr := colorAge(w, ages[i], age, err)
					ui.Item(w, f, ageStr)
				}
			}

			// Local password stored?
			lp, _ := secrets.LoadStoredLocalPassword(uid)
			lpStatus := ui.Gray(w, "not stored")
			if lp != "" {
				lpStatus = ui.Green(w, "stored")
			}
			fmt.Fprintln(w)
			ui.Header(w, "Session")
			ui.Item(w, "Local password", lpStatus)

			return nil
		},
	}
}

func unloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Emit unset commands for all tracked variables",
		Long: `Emit shell unset commands for all variables exported by renv resolve.

The output must be evaluated by your shell:

  eval "$(renv unload)"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("unloading tracked variables", "uid", uid)
			names, err := secrets.LoadVarNames(uid)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				ui.Warn(os.Stderr, "No tracked variables to unload")
				return nil
			}
			entries := make([]env.EnvEntry, len(names))
			for i, name := range names {
				entries[i] = env.EnvEntry{Key: name}
			}
			if err := env.EmitUnload(os.Stdout, entries); err != nil {
				return err
			}
			_ = secrets.ClearVarNames(uid)

			// Feedback to stderr — stdout must stay clean for eval.
			ui.Success(os.Stderr, fmt.Sprintf("Unloaded %s", ui.Bold(os.Stderr, pluralVars(len(names)))))
			ui.List(os.Stderr, names)
			return nil
		},
	}
}

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for sleep/lock events and manage secrets (run in background by shell-init)",
		Long: `Run in the background to manage secrets when the system sleeps or the screen
is locked. Normally started automatically by shell-init.

On lock: secret environment variables are unloaded from open shells. The
encrypted cache is kept so secrets can be quickly re-resolved after unlock
without re-entering passwords.

On sleep: the encrypted cache, stored session, and local passwords are cleared,
requiring full re-authentication after wake.

On Linux, sleep and screen-lock events are detected via D-Bus (systemd-logind).
The watcher listens for org.freedesktop.login1.Session.Lock and
org.freedesktop.login1.Manager.PrepareForSleep signals.

Wayland screen lockers (swaylock, waylock) must be triggered via
'loginctl lock-session' for the Lock signal to reach renv. When the locker
is invoked directly (e.g. 'exec swaylock') logind is not informed, so renv
does not receive the Lock signal and open shells are not told to unload
secret variables. The recommended swayidle configuration is:

  exec swayidle -w \
      timeout 300 'loginctl lock-session' \
      lock 'swaylock -f' \
      before-sleep 'loginctl lock-session'

  # sway keybind (e.g. in ~/.config/sway/config)
  bindsym $mod+l exec loginctl lock-session

On macOS, sleep is detected via timer drift; screen lock requires a launchd agent.
On Windows, event hooks are not yet implemented.

Start manually:
  renv watch &`,
		RunE: func(cmd *cobra.Command, args []string) error {
			detachFromTerminal()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("starting renv watcher", "uid", uid)

			hook := cleanup.New()

			// On lock: unload secret variables from open shells.
			// The cache is kept so secrets can be re-resolved after unlock
			// without re-entering passwords.
			if err := hook.RegisterLock(func() error {
				slog.Debug("cleanup: unloading renv variables on lock")
				_ = secrets.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			// On sleep: clear the encrypted cache and all stored credentials.
			// This forces full re-authentication after wake.
			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing renv cache and session on sleep")
				cache := secrets.NewCache()
				_ = cache.Clear(uid)
				_ = secrets.ClearStoredSession(uid)
				_ = secrets.ClearStoredLocalPassword(uid)
				_ = secrets.RequestUnload(uid)
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

// pluralVars returns "1 variable" or "N variables".
func pluralVars(n int) string {
	if n == 1 {
		return "1 variable"
	}
	return fmt.Sprintf("%d variables", n)
}

// colorAge colors an age string green (fresh), yellow (mid), or red (near expiry).
func colorAge(w io.Writer, raw string, d time.Duration, parseErr error) string {
	if parseErr != nil {
		return raw
	}
	switch {
	case d < 30*time.Minute:
		return ui.Green(w, raw)
	case d < 4*time.Hour:
		return ui.Yellow(w, raw)
	default:
		return ui.Red(w, raw)
	}
}
