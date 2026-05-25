package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/opalbolt/envoke/internal/cleanup"
	"github.com/opalbolt/envoke/internal/config"
	"github.com/opalbolt/envoke/internal/env"
	"github.com/opalbolt/envoke/internal/ctx"
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
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/envoke/config.yaml)")
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
		configCmd(&cfg),
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

			var ctxEntries []env.RawEntry
			var envEntries []env.RawEntry
			var defaultGroup string
			for _, e := range rawEntries {
				switch {
				case ctx.IsKCTXDirective(e.Key):
					return ctx.MigrationError(e.Key, e.Value)
				case ctx.IsCTXDirective(e.Key):
					ctxEntries = append(ctxEntries, e)
				case e.Key == "ENVOKE_DEFAULT_GROUP":
					defaultGroup = strings.ToLower(e.Value) // consumed, not exported
				default:
					envEntries = append(envEntries, e)
				}
			}


			sharedReg := newRegistry(cfg)
			defer sharedReg.Close() //nolint:errcheck // best-effort session cleanup
			dotenvDir := filepath.Dir(file)
			cp := providers.NewConfigProvider(dotenvDir, env.RenderConfigTemplate)
			sharedReg.Register(cp)
			cp.SetRegistry(sharedReg)

			
			// ── CTX_ group processing ────────────────────────────────────────────────
			// uid is shared across CTX_, kctx, and var-tracking blocks below.
			uid := fmt.Sprintf("%d", os.Getuid())
			var ctxPanelEntries []ui.PanelEntry
			ctxGroupsState := ctx.GroupsState{Groups: make(map[string][]ctx.ContextEntry)}
			// createdTmpfiles tracks CTX tmpfiles so we can clean up on error.
			var createdTmpfiles []string
			cleanupCTXTmpfiles := func() {
				for _, p := range createdTmpfiles {
					_ = os.Remove(p)
				}
			}

			if len(ctxEntries) > 0 {
				for _, e := range ctxEntries {
					entry, err := ctx.ParseContextEntry(e.Key, e.Value)
					if err != nil {
						return err
					}
					
					val, err := sharedReg.Resolve(entry.SourceURI)
					if err != nil {
						if errors.Is(err, bw.ErrInvalidPassword) {
							cleanupCTXTmpfiles()
							return err
						}
						cleanupCTXTmpfiles()
						return fmt.Errorf("resolving %s (%s): %w", e.Key, entry.SourceURI, err)
					}
					
					content := []byte(val)
					if err := ctx.ValidateContent(entry.EnvVar, content); err != nil {
						cleanupCTXTmpfiles()
						return fmt.Errorf("%s: %w", e.Key, err)
					}
					
					tmpf, err := kubeconfig.NewTempFile("envoke-ctx")
					if err != nil {
						cleanupCTXTmpfiles()
						return fmt.Errorf("creating tmpfile for %s: %w", e.Key, err)
					}
					createdTmpfiles = append(createdTmpfiles, tmpf.Name())
					if _, err := tmpf.Write(content); err != nil {
						tmpf.Close()
						cleanupCTXTmpfiles()
						return fmt.Errorf("writing tmpfile for %s: %w", e.Key, err)
					}
					if err := tmpf.Close(); err != nil {
						cleanupCTXTmpfiles()
						return fmt.Errorf("closing tmpfile for %s: %w", e.Key, err)
					}
					entry.TmpfilePath = tmpf.Name()
					
					ctxGroupsState.Groups[entry.Group] = append(ctxGroupsState.Groups[entry.Group], entry)
					ctxPanelEntries = append(ctxPanelEntries, ui.PanelEntry{
						Key:    entry.Group + ":" + entry.EnvVar,
						Source: entry.SourceURI,
					})
					slog.Debug("ctx entry resolved", "group", entry.Group, "envVar", entry.EnvVar, "tmpfile", entry.TmpfilePath)
				}
				
				if err := ctx.SaveState(uid, ctxGroupsState); err != nil {
					slog.Warn("saving ctx state", "err", err)
				}
				
				// Emit META exports as always-on baseline.
				if metaEntries, ok := ctxGroupsState.Groups["meta"]; ok {
					for _, entry := range metaEntries {
						fmt.Fprintf(os.Stdout, "export %s=%s\n", entry.EnvVar, entry.TmpfilePath)
					}
				}
				
				// Auto-apply ENVOKE_DEFAULT_GROUP if specified and the group was loaded.
				if defaultGroup != "" {
					if groupEntries, ok := ctxGroupsState.Groups[defaultGroup]; ok {
						metaEnvVars := make(map[string]bool)
						for _, e := range ctxGroupsState.Groups["meta"] {
							metaEnvVars[e.EnvVar] = true
						}
						for _, e := range groupEntries {
							if metaEnvVars[e.EnvVar] {
								fmt.Fprintf(os.Stderr, "⚠  envoke: %s defined in both meta and %s — %s wins\n", e.EnvVar, defaultGroup, defaultGroup)
							}
							fmt.Fprintf(os.Stdout, "export %s=%s\n", e.EnvVar, e.TmpfilePath)
						}
						ctxGroupsState.ActiveGroup = defaultGroup
						if err := ctx.SaveState(uid, ctxGroupsState); err != nil {
							slog.Warn("saving active group after default", "err", err)
						}
						slog.Debug("applied default group", "group", defaultGroup)
					} else {
						fmt.Fprintf(os.Stderr, "⚠  envoke: ENVOKE_DEFAULT_GROUP=%q is not a loaded group — ignored\n", defaultGroup)
					}
				}
			}
			if defaultGroup != "" && len(ctxEntries) == 0 {
				fmt.Fprintf(os.Stderr, "⚠  envoke: ENVOKE_DEFAULT_GROUP=%q set but no CTX_ groups were loaded\n", defaultGroup)
			}
			var kubeconfigPath string

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
				// uid already declared above
				names := make([]string, len(resolvedEntries))
				for i, e := range resolvedEntries {
					names[i] = e.Key
				}
				_ = state.SaveVarNames(uid, names)
			}

			panelEntries := make([]ui.PanelEntry, 0, len(resolvedEntries)+len(ctxPanelEntries))
			for _, e := range resolvedEntries {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: e.Key, Source: e.Source})
			}
			for _, e := range ctxPanelEntries {
				panelEntries = append(panelEntries, e)
			}
			totalCount := len(resolvedEntries) + len(ctxPanelEntries)
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
					if kubeconfigPath != "" || len(ctxEntries) > 0 {
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
			dotenvDir := filepath.Dir(file)
			cp := providers.NewConfigProvider(dotenvDir, env.RenderConfigTemplate)
			reg.Register(cp)
			cp.SetRegistry(reg)

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
			fmt.Fprintln(os.Stderr, "envoke load is deprecated and has been removed.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Use CTX_<GROUP>=<uri>#<ENVVAR> entries in your .env file instead:")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "  CTX_PROD=bw://kubernetes/prod#KUBECONFIG")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, `Then run: eval "$(envoke resolve .env)"`)
			return fmt.Errorf("deprecated: use CTX_ directives and envoke resolve")
		},
	}
}

// ── switch ────────────────────────────────────────────────────────────────────

func switchCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <group>",
		Short: "Switch to a context group (instant, no provider calls)",
		Long: `Switch all context env vars to a named group previously loaded by 'envoke resolve'.

No provider calls are made — all secrets were fetched eagerly by resolve and
written to tmpfiles in /dev/shm. Switch is purely local.

The META group (CTX_META) is re-applied as a persistent baseline on every switch.
'envoke switch meta' is rejected — META is always active.

Examples:
  envoke switch prod
  envoke switch staging`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.ToLower(args[0])

			if name == "meta" {
				return fmt.Errorf("'meta' is not a switchable group — META is always applied as a baseline on every switch")
			}

			uid := fmt.Sprintf("%d", os.Getuid())
			st, err := ctx.LoadState(uid)
			if err != nil {
				return fmt.Errorf("loading ctx state: %w", err)
			}
			if len(st.Groups) == 0 {
				return fmt.Errorf("no context groups loaded\n\nRun: eval \"$(envoke resolve .env)\"")
			}

			targetEntries, ok := st.Groups[name]
			if !ok {
				available := make([]string, 0, len(st.Groups))
				for g := range st.Groups {
					if g != "meta" {
						available = append(available, g)
					}
				}
				sort.Strings(available)
				return fmt.Errorf("group %q not found in loaded state\n\nAvailable groups: %s", name, strings.Join(available, ", "))
			}

			// Unload currently active group env vars (not META).
			if st.ActiveGroup != "" && st.ActiveGroup != name {
				for _, e := range st.Groups[st.ActiveGroup] {
					fmt.Fprintf(os.Stdout, "unset %s\n", e.EnvVar)
				}
			}

			// Re-apply META baseline.
			for _, e := range st.Groups["meta"] {
				fmt.Fprintf(os.Stdout, "export %s=%s\n", e.EnvVar, e.TmpfilePath)
			}

			// Build META env var set for collision detection.
			metaEnvVars := make(map[string]bool)
			for _, e := range st.Groups["meta"] {
				metaEnvVars[e.EnvVar] = true
			}

			// Apply target group — warn loudly on META collision, group wins.
			var panelEntries []ui.PanelEntry
			for _, e := range targetEntries {
				if metaEnvVars[e.EnvVar] {
					fmt.Fprintf(os.Stderr, "\u26a0  envoke: %s defined in both meta and %s — %s wins\n", e.EnvVar, name, name)
				}
				fmt.Fprintf(os.Stdout, "export %s=%s\n", e.EnvVar, e.TmpfilePath)
				panelEntries = append(panelEntries, ui.PanelEntry{
					Key:   e.EnvVar,
					Value: e.TmpfilePath,
				})
			}

			fmt.Fprintf(os.Stdout, "trap 'eval \"$(envoke unload 2>/dev/null)\"' EXIT\n")

			// Persist active group for status/unload.
			st.ActiveGroup = name
			if err := ctx.SaveState(uid, st); err != nil {
				slog.Warn("saving active group", "err", err)
			}

			headline := fmt.Sprintf("Switched to %s", ui.Bold(os.Stderr, name))
			ui.Panel(os.Stderr, "envoke", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
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
		Short: "Show loaded env vars and context groups",
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

			// ── CTX groups section ────────────────────────────────────────────
			ctxSt, ctxErr := ctx.LoadState(uid)
			if ctxErr == nil && len(ctxSt.Groups) > 0 {
				ui.Header(w, "Context groups (CTX_)")

				// META first
				if metaEntries, ok := ctxSt.Groups["meta"]; ok {
					fmt.Fprintf(w, "  ◆ meta  %s\n", ui.Gray(w, "(persistent baseline)"))
					for _, e := range metaEntries {
						fmt.Fprintf(w, "      %-32s %s\n", e.EnvVar, ui.Gray(w, e.TmpfilePath))
					}
				}

				// renderGroup prints one group's entries (path only, no extra annotations).
				renderGroup := func(entries []ctx.ContextEntry) {
					for _, e := range entries {
						fmt.Fprintf(w, "      %-32s %s\n", e.EnvVar, e.TmpfilePath)
					}
				}

				// Active group
				if ctxSt.ActiveGroup != "" {
					if entries, ok := ctxSt.Groups[ctxSt.ActiveGroup]; ok {
						fmt.Fprintf(w, "  ● %s  %s\n", ctxSt.ActiveGroup, ui.Green(w, "(active)"))
						renderGroup(entries)
					}
				}

				// Inactive groups
				groupNames := make([]string, 0, len(ctxSt.Groups))
				for g := range ctxSt.Groups {
					if g != "meta" && g != ctxSt.ActiveGroup {
						groupNames = append(groupNames, g)
					}
				}
				sort.Strings(groupNames)
				for _, g := range groupNames {
					fmt.Fprintf(w, "  ○ %s  %s\n", g, ui.Gray(w, "(inactive)"))
					renderGroup(ctxSt.Groups[g])
				}

				// Active variables summary: META + active group, showing source group.
				// Only shown when a group is active.
				if ctxSt.ActiveGroup != "" {
					fmt.Fprintln(w)
					ui.Header(w, "Active context variables")
					// Collect in order: META entries first, then active group (group wins on collision).
					seen := make(map[string]string) // envVar → group
					var order []string
					addEntries := func(entries []ctx.ContextEntry, group string) {
						for _, e := range entries {
							if _, exists := seen[e.EnvVar]; !exists {
								order = append(order, e.EnvVar)
							}
							seen[e.EnvVar] = group
						}
					}
					addEntries(ctxSt.Groups["meta"], "meta")
					addEntries(ctxSt.Groups[ctxSt.ActiveGroup], ctxSt.ActiveGroup)
					for _, envVar := range order {
						grp := seen[envVar]
						var grpLabel string
						if grp == "meta" {
							grpLabel = ui.Gray(w, "◆ meta")
						} else {
							grpLabel = ui.Green(w, "● "+grp)
						}
						fmt.Fprintf(w, "  %-32s %s\n", envVar, grpLabel)
					}
				}
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
			// ── CTX group cleanup ────────────────────────────────────────────
			ctxSt, ctxErr := ctx.LoadState(uid)
			if ctxErr == nil && len(ctxSt.Groups) > 0 {
				for grp, entries := range ctxSt.Groups {
					for _, e := range entries {
						fmt.Fprintf(os.Stdout, "unset %s\n", e.EnvVar)
						if e.TmpfilePath != "" {
							if rmErr := os.Remove(e.TmpfilePath); rmErr != nil && !os.IsNotExist(rmErr) {
								slog.Warn("removing ctx tmpfile", "path", e.TmpfilePath, "err", rmErr)
							}
						}
						panelEntries = append(panelEntries, ui.PanelEntry{
							Key:   "ctx:" + grp + ":" + e.EnvVar,
							Value: e.TmpfilePath,
						})
					}
				}
				if err := ctx.ClearState(uid); err != nil {
					slog.Warn("clearing ctx state", "err", err)
				}
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
    unload) envoke_eval unload "${@:2}" ;;
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
  [ -f "$f" ] || return 1
  stat -c '%Y:%i:%s' "$f" 2>/dev/null || stat -f '%m:%i:%z' "$f" 2>/dev/null || echo "exists"
}
_envoke_check_unload() {
  local t; t="$(_envoke_unload_token)" || return 0
  [ "${_ENVOKE_LAST_UNLOAD_TOKEN:-}" = "$t" ] && return 0
  _ENVOKE_LAST_UNLOAD_TOKEN="$t"
  eval "$(command envoke unload 2>/dev/null | grep -v '^trap ')"
}
_ENVOKE_LAST_UNLOAD_TOKEN="$(_envoke_unload_token 2>/dev/null || true)"
[ -n "${ZSH_VERSION:-}" ] && { autoload -Uz add-zsh-hook 2>/dev/null; add-zsh-hook precmd _envoke_check_unload; } || PROMPT_COMMAND="_envoke_check_unload${PROMPT_COMMAND:+; $PROMPT_COMMAND}"
[ -z "${_ENVOKE_WATCH_PID:-}" ] && { command envoke watch & _ENVOKE_WATCH_PID=$!; trap 'eval "$(command envoke unload 2>/dev/null | grep -v '"'"'^trap '"'"')"; kill "${_ENVOKE_WATCH_PID:-}" 2>/dev/null; command envoke clear-cache 2>/dev/null' EXIT; }
`

// fishCombinedInitScript is the combined shell snippet for fish.
const fishCombinedInitScript = `
function envoke
  switch $argv[1]
    case resolve; envoke_eval resolve $argv[2..]
    case unload; envoke_eval unload $argv[2..]
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
  set -l f "{{SECUREDIR}}/envoke-(id -u)-unload-requested"
  test -f "$f"; or return 1
  stat -c '%Y:%i:%s' "$f" 2>/dev/null; or stat -f '%m:%i:%z' "$f" 2>/dev/null; or echo "exists"
end
function _envoke_check_unload --on-event fish_prompt
  set -l t (_envoke_unload_token); or return
  test "$_ENVOKE_LAST_UNLOAD_TOKEN" = "$t"; and return
  set -g _ENVOKE_LAST_UNLOAD_TOKEN $t
  command envoke unload 2>/dev/null | grep -v '^trap ' | source 2>/dev/null; or true
end
function _envoke_cleanup --on-event fish_exit
  if test -n "$_ENVOKE_WATCH_PID"
    kill -0 "$_ENVOKE_WATCH_PID" 2>/dev/null; and kill "$_ENVOKE_WATCH_PID" 2>/dev/null; or true
    set -e _ENVOKE_WATCH_PID
  end
  command envoke unload 2>/dev/null | grep -v '^trap ' | source 2>/dev/null; or true
  command envoke clear-cache >/dev/null 2>/dev/null; or true
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
			kubeconfig.ClearRendered()

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
				kubeconfig.ClearRendered()
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
				kubeconfig.ClearRendered()
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

// ── config ────────────────────────────────────────────────────────────────────

func configCmd(cfg *config.Config) *cobra.Command {
	var doInit bool
	var force bool

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show configuration help or initialise a config file",
		Long: `Display envoke configuration documentation.

Config file location:
  $XDG_CONFIG_HOME/envoke/config.yaml
  (typically ~/.config/envoke/config.yaml)

Override with the --config flag:
  envoke --config /path/to/config.yaml <command>

Environment variable overrides (ENVOKE_*):
  ENVOKE_LOG_LEVEL        Log level: debug, info, warn, error
  ENVOKE_LOG_FORMAT       Log format: text or json
  ENVOKE_CACHE_MAX_AGE    Cache TTL (Go duration, e.g. 8h)
  ENVOKE_TIMEOUT_SECRETS  Secret manager CLI timeout (e.g. 30s)
  ENVOKE_UI_BORDER        Show UI border: true or false
  ENVOKE_BW_PASSWORD      Bitwarden master password (skips interactive prompt)

Flags:
  --init    Write a default commented config file to the config path`,
		// Skip root's PersistentPreRunE (config.Load) so a malformed config
		// file doesn't block "envoke config --init --force" from fixing it.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if !doInit {
				return cmd.Help()
			}

			// Determine target path
			var path string
			if cfgFile := cmd.Root().PersistentFlags().Lookup("config"); cfgFile != nil && cfgFile.Value.String() != "" {
				path = cfgFile.Value.String()
			} else {
				path = config.DefaultConfigFile()
			}

			// Create parent directory if needed
			dir := filepath.Dir(path)
			if dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					return fmt.Errorf("creating directory %s: %w", dir, err)
				}
			}

			// Check if file exists
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("config file already exists: %s (use --force to overwrite)", path)
			}

			// Write template
			if err := os.WriteFile(path, []byte(defaultConfigTemplate), 0o600); err != nil {
				return fmt.Errorf("writing config file: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&doInit, "init", false, "Write a default commented config file to the config path")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config file")
	return cmd
}

const defaultConfigTemplate = `# envoke configuration
#
# Default location: $XDG_CONFIG_HOME/envoke/config.yaml
#                   (~/.config/envoke/config.yaml if XDG_CONFIG_HOME is unset)
#
# Override path with: envoke --config /path/to/config.yaml <command>
#
# Precedence (highest to lowest):
#   CLI flags  >  environment variables (ENVOKE_*)  >  this file  >  built-in defaults
#
# Authentication
# --------------
# envoke uses your Bitwarden master password to unlock the vault on first access.
# Supply it non-interactively via environment variable to avoid TTY prompts:
#
#   ENVOKE_BW_PASSWORD  — master password, piped to bw unlock; never written to disk
#   BW_SESSION          — pre-existing Bitwarden session token (skips bw unlock entirely)

log:
  # Minimum log level written to stderr.
  # Values: debug | info | warn | error
  # Env:    ENVOKE_LOG_LEVEL
  # Flag:   --log-level, --verbose (shorthand for debug)
  level: warn

  # Log output format.
  # Values: text | json
  # Env:    ENVOKE_LOG_FORMAT
  format: text

cache:
  # Maximum age of cached Bitwarden folder/collection items.
  # After this TTL expires envoke discards the local cache and re-fetches from
  # Bitwarden (requiring your master password or BW_SESSION again).
  # Accepts Go duration strings: 1h, 8h, 24h, etc.
  # Env: ENVOKE_CACHE_MAX_AGE
  max_age: 8h

timeouts:
  # Timeout applied to each Bitwarden CLI (bw) subprocess call.
  # Covers: bw status, bw unlock, bw list folders/items/collections.
  # Accepts Go duration strings: 10s, 30s, 1m, etc.
  # Env: ENVOKE_TIMEOUT_SECRETS
  secrets: 30s

ui:
  # Show a rounded border on the secrets-loaded/unloaded summary panel.
  # Set to false for minimal output or when piping stderr.
  # Env: ENVOKE_UI_BORDER
  border: true
`
