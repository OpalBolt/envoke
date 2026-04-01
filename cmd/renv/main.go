package main

import (
	"fmt"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var verbose bool
	var noCache bool

	root := &cobra.Command{
		Use:   "renv",
		Short: "Resolve secret references in .env and YAML files",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				_ = verbose // TODO: wire to logger
			}
		},
	}

	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")
	root.PersistentFlags().BoolVar(&noCache, "no-cache", false, "Disable encrypted cache")

	root.AddCommand(
		resolveCmd(&noCache),
		yamlCmd(),
		clearCacheCmd(),
		statusCmd(),
		versionCmd(),
		unloadCmd(),
	)
	return root
}

func resolveCmd(noCache *bool) *cobra.Command {
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
			if term.IsTerminal(int(os.Stdout.Fd())) {
				fmt.Fprintln(os.Stderr, "renv: warning: stdout is a terminal — output will not be set as env vars.")
				fmt.Fprintln(os.Stderr, "renv: use: eval \"$(renv resolve .env)\"")
			}
			cache := secrets.NewCache()
			if *noCache {
				cache.Disabled = true // bypass both Put and Get — no secrets touch disk
			}
			bwClient := &secrets.BWClient{Cache: cache}
			vaultClient := &secrets.VaultClient{}

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

func yamlCmd() *cobra.Command {
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
			cache := secrets.NewCache()
			bwClient := &secrets.BWClient{Cache: cache}
			vaultClient := &secrets.VaultClient{}

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
			if err := cache.Clear(uid); err != nil {
				return fmt.Errorf("clearing cache: %w", err)
			}
			if err := secrets.ClearStoredSession(uid); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}
			// Var-name tracking is not cleared here — that is renv unload's job.
			// Keeping the names file intact ensures renv unload remains functional
			// even when clear-cache is triggered by the shell EXIT trap.
			fmt.Fprintln(os.Stderr, "renv: cache cleared")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cache status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := secrets.NewCache()
			files, ages, err := secrets.CacheStatus(cache)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				fmt.Println("No cache files found.")
				return nil
			}
			for i, f := range files {
				fmt.Printf("%s (age: %s)\n", f, ages[i])
			}
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("renv %s\n", version)
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
			names, err := secrets.LoadVarNames(uid)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Fprintln(os.Stderr, "renv: no tracked variables to unload")
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
			return nil
		},
	}
}
