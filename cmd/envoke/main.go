package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/opalbolt/envoke/internal/cleanup"
	"github.com/opalbolt/envoke/internal/config"
	"github.com/opalbolt/envoke/internal/env"
	"github.com/opalbolt/envoke/internal/kubeconfig"
	"github.com/opalbolt/envoke/internal/logger"
	"github.com/opalbolt/envoke/internal/providers"
	bw "github.com/opalbolt/envoke/internal/providers/bitwarden"
	"github.com/opalbolt/envoke/internal/securedir"
	"github.com/opalbolt/envoke/internal/state"
	"github.com/opalbolt/envoke/internal/ui"
	"github.com/opalbolt/envoke/internal/version"
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
		Use:   "envoke",
		Short: "Unified secret environment loader — env vars and kubeconfigs",
		Long: `envoke (env + invoke) resolves secrets and kubeconfigs from a single .env file.

  envoke resolve .env            # resolve both env secrets and kubeconfig refs
  envoke switch prod             # switch KUBECONFIG to a pre-loaded named config
  envoke shell-init              # combined shell setup

The .env file supports KCTX_<name>=bw://... entries that load kubeconfigs into the local named store.`,
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
			)
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	root.AddCommand(
		resolveCmd(&cfg),
		execCmd(&cfg),
		yamlCmd(&cfg),
		loadCmd(&cfg),
		switchCmd(&cfg),
		unloadCmd(&cfg),
		statusCmd(),
		shellInitCmd(),
		clearCacheCmd(),
		watchCmd(),
	)
	return root
}

// newRegistry builds a secrets Registry with the Bitwarden provider.
func newRegistry(cfg *config.Config) *providers.Registry {
	bwClient := &bw.BWClient{
		Timeout: cfg.SecretsTimeout(),
	}

	reg := providers.NewRegistry()
	reg.Register(providers.NewBWProvider(bwClient))
	return reg
}

// normalizeKubeconfigURI normalises a kubeconfig source URI.
func normalizeKubeconfigURI(source string) string {
	return kubeconfig.NormalizeSourceURI(source)
}

// ── resolve ───────────────────────────────────────────────────────────────────

func resolveCmd(cfg *config.Config) *cobra.Command {
	var file string
	var shell string
	var force bool

	cmd := &cobra.Command{
		Use:   "resolve [file]",
		Short: "Resolve .env secrets and kubeconfig directives",
		Long: `Resolve a .env file, handling both secret references and kubeconfig directives.

Secret references (bw://) are resolved and exported as shell variables.

Kubeconfig directives (keys prefixed with KCTX_) load a kubeconfig into the
local named store and automatically set KUBECONFIG to the last loaded one:

  KCTX_PROD=bw://kubernetes/prod-cluster     # loads kubeconfig named "prod"

Use 'envoke switch <name>' to switch between loaded kubeconfigs at any time.

The output must be evaluated by your shell:

  eval "$(envoke resolve .env)"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				file = args[0]
			}
			slog.Debug("running envoke resolve", "file", file)
			if term.IsTerminal(int(os.Stdout.Fd())) && !force {
				return fmt.Errorf("stdout is a terminal — output will not be set as env vars\n\n" +
					"Wrap the command with eval to apply the exports to your shell:\n\n" +
					"  eval \"$(envoke resolve .env)\"\n\n" +
					"Use --force to override this check and print to terminal anyway.")
			}

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

			for _, e := range kctxEntries {
				if err := kubeconfig.ValidateStoreName(kctxNameFromKey(e.Key)); err != nil {
					return fmt.Errorf("invalid kctx directive %q: %w", e.Key, err)
				}
			}

			sharedReg := newRegistry(cfg)
			defer sharedReg.Close() //nolint:errcheck // best-effort session cleanup

			var kctxPanelEntries []ui.PanelEntry
			var kubeconfigPath string
			if len(kctxEntries) > 0 {
				uid := fmt.Sprintf("%d", os.Getuid())
				store := kubeconfig.NewNamedStore()
				var kctxNames []string
				var lastKubeconfigName string
				for _, e := range kctxEntries {
					name := kctxNameFromKey(e.Key)
					if err := fetchKubeconfigForDirective(sharedReg, name, e.Value, uid, store); err != nil {
						if errors.Is(err, bw.ErrInvalidPassword) {
							return err
						}
						return fmt.Errorf("loading kubeconfig %q (%s): %w", name, e.Value, err)
					}
					lastKubeconfigName = name
					kctxNames = append(kctxNames, name)
					kctxPanelEntries = append(kctxPanelEntries, ui.PanelEntry{
						Key:    name,
						Source: e.Value,
					})
					slog.Debug("loaded kubeconfig into store", "name", name, "source", e.Value)
				}
				_ = kubeconfig.SaveTrackedNames(uid, kctxNames)

				if lastKubeconfigName != "" {
					kubeconfigPath = store.Path(uid, lastKubeconfigName)
					slog.Debug("set KUBECONFIG", "path", kubeconfigPath)
				}
			}

			var resolvedEntries []env.EnvEntry
			if len(envEntries) > 0 {
				tmpFile, err := writeTempEnv(envEntries)
				if err != nil {
					return fmt.Errorf("preparing env entries: %w", err)
				}
				defer os.Remove(tmpFile)

				resolvedEntries, err = env.ResolveDotEnv(tmpFile, sharedReg)
				if err != nil {
					if errors.Is(err, bw.ErrInvalidPassword) {
						return err
					}
					return fmt.Errorf("resolving %s: %w", file, err)
				}
			}

			if err := env.EmitExports(os.Stdout, resolvedEntries); err != nil {
				return err
			}
			if kubeconfigPath != "" {
				fmt.Fprintf(os.Stdout, "export KUBECONFIG=%s\n", kubeconfigPath)
			}

			if len(resolvedEntries) > 0 {
				uid := fmt.Sprintf("%d", os.Getuid())
				names := make([]string, len(resolvedEntries))
				for i, e := range resolvedEntries {
					names[i] = e.Key
				}
				_ = state.SaveVarNames(uid, names)
			}

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
					if kubeconfigPath != "" {
						fmt.Println("trap 'eval \"$(envoke unload 2>/dev/null)\" 2>/dev/null; envoke clear-cache 2>/dev/null' EXIT")
					} else {
						fmt.Println("trap 'envoke clear-cache' EXIT")
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", ".env", "Path to .env file")
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type (bash|fish|zsh)")
	cmd.Flags().BoolVar(&force, "force", false, "Bypass terminal check and print exports to terminal anyway")
	return cmd
}

// isKctxDirective returns true if the entry is a KCTX_<name>=bw:// directive.
func isKctxDirective(e env.RawEntry) bool {
	if !strings.HasPrefix(e.Key, "KCTX_") {
		return false
	}
	return strings.HasPrefix(e.Value, "bw://")
}

// kctxNameFromKey derives a kubeconfig store name from a KCTX_ key.
// KCTX_PROD → "prod", KCTX_MY_CLUSTER → "my_cluster"
func kctxNameFromKey(key string) string {
	return strings.ToLower(strings.TrimPrefix(key, "KCTX_"))
}

// fetchKubeconfigForDirective fetches a kubeconfig from a bw:// source and stores it.
func fetchKubeconfigForDirective(reg *providers.Registry, name, source, uid string, store *kubeconfig.NamedStore) error {
	uri := normalizeKubeconfigURI(source)
	val, err := reg.Resolve(uri)
	if err != nil {
		return err
	}
	return store.Put(uid, name, []byte(val))
}

// writeTempEnv writes env entries to a temp .env file for processing by ResolveDotEnv.
func writeTempEnv(entries []env.RawEntry) (string, error) {
	f, err := os.CreateTemp("", "envoke-*.env")
	if err != nil {
		return "", err
	}
	defer f.Close()
	for _, e := range entries {
		if _, err := fmt.Fprintf(f, "%s=%s\n", e.Key, e.Value); err != nil {
			return "", err
		}
	}
	return f.Name(), nil
}

func pluralItems(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

// ── exec ──────────────────────────────────────────────────────────────────────

func execCmd(cfg *config.Config) *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "exec -- command [args...]",
		Short: "Run a command with resolved env vars injected (no eval needed)",
		Long: `Resolve secret references from a .env file and execute a command with those
variables set in its environment. No eval required.

  envoke exec -- myprogram --flag value
  envoke exec --env secrets.env -- myprogram

The -- separator is required to distinguish envoke flags from the command's args.
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

// ── load ──────────────────────────────────────────────────────────────────────

func loadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "load <name> <bw://item>",
		Short: "Fetch a kubeconfig and cache it under a local name",
		Long: `Fetch a kubeconfig from Bitwarden and store it in the local
named store so that 'envoke switch <name>' can load it without re-fetching.

Examples:
  envoke load prod bw://kubernetes/prod`,
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
			defer reg.Close() //nolint:errcheck // best-effort session cleanup

			uri := normalizeKubeconfigURI(source)
			val, err := reg.Resolve(uri)
			if err != nil {
				return err
			}

			if err := store.Put(uid, name, []byte(val)); err != nil {
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
		Use:   "switch <name> [bw://item]",
		Short: "Switch to a named kubeconfig (or fetch one if a source is given)",
		Long: `Switch KUBECONFIG to a named kubeconfig previously loaded with 'envoke load'.

If the named kubeconfig is not in the local store, a source (bw://) may be
provided to fetch it on the fly.

Examples:
  envoke switch prod                          # use pre-loaded 'prod'
  envoke switch staging bw://k8s/staging      # fetch directly if not pre-loaded`,
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
							"Run: envoke load %s <bw://item>",
						name, name,
					)
				}
				reg := newRegistry(cfg)
				defer reg.Close() //nolint:errcheck // best-effort session cleanup
				uri := normalizeKubeconfigURI(source)
				val, err := reg.Resolve(uri)
				if err != nil {
					return fmt.Errorf("fetching kubeconfig for %q: %w", name, err)
				}
				kubeconfigData = []byte(val)
				if err := store.Put(uid, name, kubeconfigData); err != nil {
					return fmt.Errorf("caching kubeconfig %q: %w", name, err)
				}
			}

			path := store.Path(uid, name)

			fmt.Printf("export KUBECONFIG=%s\n", path)
			fmt.Printf("trap 'eval \"$(envoke unload 2>/dev/null)\"' EXIT\n")

			srcLabel := switchSourceLabel(name, source)
			panelEntries := []ui.PanelEntry{
				{Key: "KUBECONFIG", Value: path, Source: srcLabel},
			}
			if ctx := currentKubectlContext(path); ctx != "" {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: "Context", Value: ctx})
			}
			headline := fmt.Sprintf("Switched to %s", ui.Bold(os.Stderr, name))
			ui.Panel(os.Stderr, "envoke", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
}

func switchSourceLabel(name, source string) string {
	if source == "" {
		return "named store"
	}
	return source
}

func currentKubectlContext(kubeconfigPath string) string {
	cmd := exec.Command("kubectl", "config", "current-context")
	if kubeconfigPath != "" {
		environ := make([]string, 0, len(os.Environ())-1)
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "KUBECONFIG=") {
				environ = append(environ, e)
			}
		}
		cmd.Env = append(environ, "KUBECONFIG="+kubeconfigPath)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ── status ────────────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show loaded env vars, KUBECONFIG, and named kubeconfigs",
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

			ui.Header(w, "Kubeconfig")
			kc := os.Getenv("KUBECONFIG")
			if kc == "" {
				ui.Item(w, "KUBECONFIG", ui.Gray(w, "not set"))
				ui.Item(w, "Managed by envoke", ui.Gray(w, "no"))
			} else {
				ui.Item(w, "KUBECONFIG", ui.Green(w, kc))
				if kubeconfig.IsManaged(kc) {
					ui.Item(w, "Managed by envoke", ui.Green(w, "yes"))
				} else {
					ui.Item(w, "Managed by envoke", ui.Yellow(w, "no (external)"))
				}
				if ctx := currentKubectlContext(kc); ctx != "" {
					ui.Item(w, "Current context", ui.Bold(w, ctx))
				}
			}

			store := kubeconfig.NewNamedStore()
			storedNames, err := store.List(uid)
			if err != nil {
				slog.Warn("listing named kubeconfigs", "err", err)
			} else if len(storedNames) > 0 {
				sort.Strings(storedNames)
				ui.Header(w, "Loaded kubeconfigs (use 'envoke switch <name>')")
				ui.List(w, storedNames)
			}
			return nil
		},
	}
}

// ── unload ────────────────────────────────────────────────────────────────────

func unloadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Unset all tracked env vars and KUBECONFIG",
		Long: `Emit shell commands to unset all variables exported by envoke resolve,
and unset KUBECONFIG if it was set by envoke.

The output must be evaluated by your shell:

  eval "$(envoke unload)"

When using the envoke shell-init, the shell function handles this automatically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("unloading all envoke state", "uid", uid)

			var panelEntries []ui.PanelEntry

			names, err := state.LoadVarNames(uid)
			if err != nil {
				slog.Warn("loading tracked var names", "err", err)
			}
			if len(names) > 0 {
				entries := make([]env.EnvEntry, len(names))
				for i, name := range names {
					entries[i] = env.EnvEntry{Key: name}
				}
				if emitErr := env.EmitUnload(os.Stdout, entries); emitErr != nil {
					slog.Warn("emitting unload", "err", emitErr)
				} else {
					_ = state.ClearVarNames(uid)
					for _, n := range names {
						panelEntries = append(panelEntries, ui.PanelEntry{Key: n})
					}
				}
			}

			kubeconfigPath := os.Getenv("KUBECONFIG")
			if kubeconfigPath != "" && kubeconfig.IsManaged(kubeconfigPath) {
				if err := os.Remove(kubeconfigPath); err != nil && !os.IsNotExist(err) {
					slog.Warn("removing managed kubeconfig", "path", kubeconfigPath, "err", err)
				}
				panelEntries = append(panelEntries, ui.PanelEntry{
					Key:   "KUBECONFIG",
					Value: kubeconfigPath,
				})
			}
			fmt.Fprintln(os.Stdout, "unset KUBECONFIG")

			kctxNames, err := kubeconfig.LoadTrackedNames(uid)
			if err != nil {
				slog.Warn("loading tracked kctx names", "err", err)
			}
			if len(kctxNames) > 0 {
				store := kubeconfig.NewNamedStore()
				for _, name := range kctxNames {
					if rmErr := store.Remove(uid, name); rmErr != nil {
						slog.Warn("removing kctx store entry", "name", name, "err", rmErr)
					} else {
						slog.Debug("removed kctx store entry", "name", name)
						panelEntries = append(panelEntries, ui.PanelEntry{
							Key:   "kctx:" + name,
							Value: "(store)",
						})
					}
				}
				_ = kubeconfig.ClearTrackedNames(uid)
			}

			if len(panelEntries) == 0 {
				ui.Warn(os.Stderr, "Nothing to unload")
				return nil
			}
			headline := fmt.Sprintf("Unloaded %s", ui.Bold(os.Stderr, pluralItems(len(panelEntries))))
			ui.Panel(os.Stderr, "envoke", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
}

// ── shell-init ────────────────────────────────────────────────────────────────

func shellInitCmd() *cobra.Command {
	var shell string
	var force bool

	cmd := &cobra.Command{
		Use:   "shell-init",
		Short: "Print shell integration functions for envoke",
		Long: `Print shell function definitions that make envoke resolve and unload
work without explicit eval, and install a combined watcher and EXIT trap.

Add to your shell config once:

  # bash / zsh (~/.bashrc or ~/.zshrc)
  eval "$(envoke shell-init)"

  # fish (~/.config/fish/config.fish)
  envoke shell-init --shell fish | source

After that:
  envoke resolve .env        # load env secrets and kubeconfigs
  envoke switch prod         # switch to 'prod' kubeconfig
  envoke unload              # unload everything`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if term.IsTerminal(int(os.Stdout.Fd())) && !force {
				return fmt.Errorf("shell-init output must not be eval'd directly in a terminal\n\n" +
					"Instead, add this to your shell config file:\n\n" +
					"  # bash/zsh (~/.bashrc or ~/.zshrc)\n" +
					"  eval \"$(envoke shell-init)\"\n\n" +
					"  # fish (~/.config/fish/config.fish)\n" +
					"  envoke shell-init --shell fish | source\n\n" +
					"Use --force to override this check.")
			}
			secureDir := securedir.Dir()
			var script string
			switch shell {
			case "fish":
				script = strings.ReplaceAll(fishCombinedInitScript, "{{SECUREDIR}}", secureDir)
			default:
				script = strings.ReplaceAll(bashCombinedInitScript, "{{SECUREDIR}}", secureDir)
			}
			_, err := io.WriteString(cmd.OutOrStdout(), script)
			return err
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type: bash, zsh, fish")
	cmd.Flags().BoolVar(&force, "force", false, "Bypass terminal check and output init script (unsafe)")
	return cmd
}

// bashCombinedInitScript is the combined shell snippet for bash/zsh.
// Defines an envoke() shell function that auto-evals resolve, unload, and switch output,
// starts a single background watcher, and installs a combined EXIT trap.
const bashCombinedInitScript = `
envoke() {
  case "$1" in
    resolve) envoke_eval resolve "${@:2}" ;;
    unload) envoke_eval unload ;;
    switch) envoke_eval switch "${@:2}"; _ENVOKE_LAST_UNLOAD_TOKEN="$(_envoke_unload_token 2>/dev/null || true)" ;;
    *) command envoke "$@" ;;
  esac
}
envoke_eval() {
  local _cmd="$1" _out _exit
  shift
  for _arg in "$@"; do case "$_arg" in --help|-h|--version) command envoke "$_cmd" "$@"; return ;; esac; done
  _out="$(command envoke "$_cmd" "$@")" _exit=$?
  [ $_exit -ne 0 ] && return $_exit
  eval "$(printf '%s\n' "$_out" | grep -v '^trap ')"
}
_envoke_unload_token() {
  local f="{{SECUREDIR}}/envoke-${UID}-unload-requested"
  [ -f "$f" ] && stat -c '%Y:%i:%s' "$f" 2>/dev/null || stat -f '%m:%i:%z' "$f" 2>/dev/null || true
}
_envoke_check_unload() {
  local t; t="$(_envoke_unload_token)" || return 0
  [ "${_ENVOKE_LAST_UNLOAD_TOKEN:-}" = "$t" ] && return 0
  _ENVOKE_LAST_UNLOAD_TOKEN="$t"
  command envoke unload 2>/dev/null | grep -v '^trap ' | eval "$(cat)"
}
_ENVOKE_LAST_UNLOAD_TOKEN="$(_envoke_unload_token 2>/dev/null || true)"
[ -n "${ZSH_VERSION:-}" ] && { autoload -Uz add-zsh-hook 2>/dev/null; add-zsh-hook precmd _envoke_check_unload; } || PROMPT_COMMAND="_envoke_check_unload${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
[ -z "${_ENVOKE_WATCH_PID:-}" ] && { command envoke watch & _ENVOKE_WATCH_PID=$!; trap 'command envoke unload 2>/dev/null | eval "$(cat)"; kill "${_ENVOKE_WATCH_PID:-}" 2>/dev/null; command envoke clear-cache 2>/dev/null' EXIT; }
`

// fishCombinedInitScript is the combined shell snippet for fish.
const fishCombinedInitScript = `
function envoke
  switch $argv[1]
    case resolve; envoke_eval resolve $argv[2..]
    case unload; envoke_eval unload
    case switch; envoke_eval switch $argv[2..]; set -g _ENVOKE_LAST_UNLOAD_TOKEN (_envoke_unload_token 2>/dev/null; or echo "")
    case '*'; command envoke $argv
  end
end
function envoke_eval
  set -l _cmd $argv[1]
  set -l _args $argv[2..]
  for _arg in $_args
    if test "$_arg" = "--help" -o "$_arg" = "-h" -o "$_arg" = "--version"; command envoke $_cmd $_args; return; end
  end
  set -l _out (command envoke $_cmd $_args); set -l _exit $status
  test $_exit -ne 0; and return $_exit
  printf '%s\n' $_out | grep -v '^trap ' | source
end
function _envoke_unload_token
  set -l f {{SECUREDIR}}/envoke-(id -u)-unload-requested
  test -f $f; and stat -c '%Y:%i:%s' $f 2>/dev/null || stat -f '%m:%i:%z' $f 2>/dev/null || echo ""
end
function _envoke_check_unload --on-event fish_prompt
  set -l t (_envoke_unload_token); or return
  test "$_ENVOKE_LAST_UNLOAD_TOKEN" = "$t"; and return
  set -g _ENVOKE_LAST_UNLOAD_TOKEN $t
  command envoke unload 2>/dev/null | grep -v '^trap ' | source 2>/dev/null; or true
end
set -g _ENVOKE_LAST_UNLOAD_TOKEN (_envoke_unload_token 2>/dev/null; or echo "")
test -n "$_ENVOKE_WATCH_PID"; or { command envoke watch &; set -gx _ENVOKE_WATCH_PID $last_pid; }
`

// ── clear-cache + watch ───────────────────────────────────────────────────────

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove stored Bitwarden session and all kubeconfig files",
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("clearing all cached data", "uid", uid)

			if err := bw.ClearStoredSession(uid); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}

			store := kubeconfig.NewNamedStore()
			if err := store.Clear(uid); err != nil {
				return fmt.Errorf("clearing kubeconfig store: %w", err)
			}

			ui.Success(os.Stderr, "Cache cleared")
			return nil
		},
	}
}

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
				slog.Debug("cleanup: unloading env variables and kubeconfigs on lock")
				_ = state.RequestUnload(uid)
				kubeconfig.ClearManaged()
				_ = kubeconfig.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing all cached data on sleep")
				_ = bw.ClearStoredSession(uid)
				_ = state.RequestUnload(uid)
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
