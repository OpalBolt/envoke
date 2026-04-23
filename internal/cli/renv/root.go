package renv

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/opalbolt/envoke/internal/cleanup"
	"github.com/opalbolt/envoke/internal/config"
	"github.com/opalbolt/envoke/internal/env"
	"github.com/opalbolt/envoke/internal/logger"
	"github.com/opalbolt/envoke/internal/providers"
	bw "github.com/opalbolt/envoke/internal/providers/bitwarden"
	"github.com/opalbolt/envoke/internal/securedir"
	"github.com/opalbolt/envoke/internal/state"
	"github.com/opalbolt/envoke/internal/ui"
	"github.com/opalbolt/envoke/internal/version"
)

// NewRootCmd returns the root cobra command for the renv CLI.
func NewRootCmd() *cobra.Command {
	var verbose bool
	var cfgFile string
	var logLevel string
	var cfg config.Config

	root := &cobra.Command{
		Use:          "renv",
		Short:        "Resolve secret references in .env and YAML files",
		Version:      version.String(),
		SilenceUsage: true,
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
			logger.Init(cfg.Log.Level, cfg.Log.Format)
			slog.Debug("config loaded",
				"log_level", cfg.Log.Level,
				"log_format", cfg.Log.Format,
				"cache_max_age", cfg.Cache.MaxAge,
				"timeout_secrets", cfg.Timeouts.Secrets,
			)
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	// Deprecated flags kept for backwards compatibility — they are now no-ops.
	var ignoredBool bool
	var ignoredString string
	root.PersistentFlags().BoolVar(&ignoredBool, "isolated", false, "Deprecated: no longer has any effect")
	root.PersistentFlags().StringVar(&ignoredString, "password-grace-period", "", "Deprecated: no longer has any effect")
	_ = root.PersistentFlags().MarkDeprecated("isolated", "this flag no longer has any effect and will be removed in a future release")
	_ = root.PersistentFlags().MarkDeprecated("password-grace-period", "this flag no longer has any effect and will be removed in a future release")

	root.AddCommand(
		resolveCmd(&cfg),
		execCmd(&cfg),
		shellInitCmd(),
		yamlCmd(&cfg),
		clearCacheCmd(),
		statusCmd(),
		unloadCmd(&cfg),
		watchCmd(),
	)
	return root
}

// NewSubCmd returns the renv subcommand tree for embedding under another root (e.g. envoke).
// Unlike NewRootCmd, it does not register persistent flags (those are inherited from the parent)
// and does not install its own PersistentPreRunE (the parent's runs instead).
// cfg is a pointer owned by the parent, populated before any subcommand runs.
func NewSubCmd(cfg *config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "renv",
		Short: "Resolve secret references in .env and YAML files",
	}
	root.AddCommand(
		resolveCmd(cfg),
		execCmd(cfg),
		shellInitCmd(),
		yamlCmd(cfg),
		clearCacheCmd(),
		statusCmd(),
		unloadCmd(cfg),
		watchCmd(),
	)
	return root
}

// ── registry ──────────────────────────────────────────────────────────────────

func newRegistry(cfg *config.Config) *providers.Registry {
	bwClient := &bw.BWClient{
		Timeout: cfg.SecretsTimeout(),
	}

	reg := providers.NewRegistry()
	reg.Register(providers.NewBWProvider(bwClient))
	return reg
}

// ── resolve ───────────────────────────────────────────────────────────────────

func resolveCmd(cfg *config.Config) *cobra.Command {
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
			reg := newRegistry(cfg)
			defer reg.Close() //nolint:errcheck // best-effort session cleanup

			entries, err := env.ResolveDotEnv(file, reg)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", file, err)
			}

			if err := env.EmitExports(os.Stdout, entries); err != nil {
				return err
			}

			uid := fmt.Sprintf("%d", os.Getuid())
			names := make([]string, len(entries))
			panelEntries := make([]ui.PanelEntry, len(entries))
			for i, e := range entries {
				names[i] = e.Key
				panelEntries[i] = ui.PanelEntry{Key: e.Key, Source: e.Source}
			}
			_ = state.SaveVarNames(uid, names)

			headline := fmt.Sprintf("Loaded %s from %s",
				ui.Bold(os.Stderr, pluralVars(len(entries))),
				ui.Bold(os.Stderr, file))
			ui.Panel(os.Stderr, "renv", headline, panelEntries, cfg.UI.Border)

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

// ── exec ──────────────────────────────────────────────────────────────────────

func execCmd(cfg *config.Config) *cobra.Command {
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
			reg := newRegistry(cfg)
			defer reg.Close() //nolint:errcheck // best-effort session cleanup

			entries, err := env.ResolveDotEnv(file, reg)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", file, err)
			}

			environ := os.Environ()
			for _, e := range entries {
				environ = append(environ, e.Key+"="+e.Value)
			}

			bin, err := exec.LookPath(args[0])
			if err != nil {
				return fmt.Errorf("%s: command not found", args[0])
			}

			return syscall.Exec(bin, args, environ)
		},
	}
	cmd.Flags().StringVarP(&file, "env", "e", ".env", "Path to .env file")
	return cmd
}

// ── shell-init ────────────────────────────────────────────────────────────────

// BashInitScript returns the shell function emitted by `renv shell-init` for bash/zsh.
// The sentinel file directory is resolved at emit time via securedir.Dir() so
// the shell does not need to probe multiple candidate paths.
// Exported so envoke can reference it when building the combined shell-init.
func BashInitScript() string {
	const tmpl = `renv() {
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
  local f="{{SECUREDIR}}/renv-${UID}-unload-requested"
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
	return strings.Replace(tmpl, "{{SECUREDIR}}", securedir.Dir(), 1)
}

// FishInitScript returns the shell function emitted by `renv shell-init --shell fish`.
// The sentinel file directory is resolved at emit time via securedir.Dir().
func FishInitScript() string {
	const tmpl = `function renv
  switch $argv[1]
    case resolve unload
      command renv $argv | source
    case '*'
      command renv $argv
  end
end

function _renv_unload_token
  set -l f {{SECUREDIR}}/renv-(id -u)-unload-requested
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
	return strings.Replace(tmpl, "{{SECUREDIR}}", securedir.Dir(), 1)
}

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
				_, err := io.WriteString(cmd.OutOrStdout(), FishInitScript())
				return err
			default:
				_, err := io.WriteString(cmd.OutOrStdout(), BashInitScript())
				return err
			}
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type: bash, zsh, fish")
	return cmd
}

// ── yaml ──────────────────────────────────────────────────────────────────────

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
			reg := newRegistry(cfg)
			defer reg.Close() //nolint:errcheck // best-effort session cleanup

			data, err := env.ResolveYAML(file, reg)
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

// ── clear-cache ───────────────────────────────────────────────────────────────

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove stored Bitwarden session",
		Long: `Remove the stored Bitwarden session.

Variable name tracking (used by renv unload) is intentionally preserved so that
renv unload continues to work after a cache clear — for example when the EXIT
trap fires inside a direnv subprocess.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("clearing session", "uid", uid)
			if err := bw.ClearStoredSession(uid); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}
			ui.Success(os.Stderr, "Session cleared")
			return nil
		},
	}
}

// ── status ────────────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show loaded variable status",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := os.Stdout
			uid := fmt.Sprintf("%d", os.Getuid())

			ui.Header(w, "Tracked variables")
			names, err := state.LoadVarNames(uid)
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

			return nil
		},
	}
}

// ── unload ────────────────────────────────────────────────────────────────────

func unloadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Emit unset commands for all tracked variables",
		Long: `Emit shell unset commands for all variables exported by renv resolve.

The output must be evaluated by your shell:

  eval "$(renv unload)"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("unloading tracked variables", "uid", uid)
			names, err := state.LoadVarNames(uid)
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
			_ = state.ClearVarNames(uid)

			panelEntries := make([]ui.PanelEntry, len(names))
			for i, n := range names {
				panelEntries[i] = ui.PanelEntry{Key: n}
			}
			headline := fmt.Sprintf("Unloaded %s", ui.Bold(os.Stderr, pluralVars(len(names))))
			ui.Panel(os.Stderr, "renv", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
}

// ── watch ─────────────────────────────────────────────────────────────────────

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for sleep/lock events and manage secrets (run in background by shell-init)",
		Long: `Run in the background to manage secrets when the system sleeps or the screen
is locked. Normally started automatically by shell-init.

On lock: secret environment variables are unloaded from open shells.

On sleep: the stored session is cleared, requiring re-authentication after wake.

On Linux, sleep and screen-lock events are detected via D-Bus (systemd-logind).
On macOS, sleep is detected via timer drift; screen lock requires a launchd agent.
On Windows, event hooks are not yet implemented.

Start manually:
  renv watch &`,
		RunE: func(cmd *cobra.Command, args []string) error {
			detachFromTerminal()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("starting renv watcher", "uid", uid)

			hook := cleanup.New()

			if err := hook.RegisterLock(func() error {
				slog.Debug("cleanup: unloading renv variables on lock")
				_ = state.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing renv session on sleep")
				_ = bw.ClearStoredSession(uid)
				_ = state.RequestUnload(uid)
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

// ── helpers ───────────────────────────────────────────────────────────────────

func pluralVars(n int) string {
	if n == 1 {
		return "1 variable"
	}
	return fmt.Sprintf("%d variables", n)
}

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
