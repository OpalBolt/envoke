package main

import (
	"fmt"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
	"github.com/spf13/cobra"
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
		shellInitCmd(),
		unloadCmd(),
	)
	return root
}

func resolveCmd(noCache *bool) *cobra.Command {
	var file string
	var shell string

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve .env file secret references and emit shell exports",
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := secrets.NewCache()
			if *noCache {
				cache.MaxAge = 0
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

			// Emit EXIT trap
			switch shell {
			case "fish":
				fmt.Println("# Fish shell trap not supported via eval; use renv clear-cache manually")
			default:
				fmt.Println("trap 'renv clear-cache' EXIT")
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
		Short: "Remove all renv cache files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := secrets.NewCache()
			uid := fmt.Sprintf("%d", os.Getuid())
			if err := cache.Clear(uid); err != nil {
				return fmt.Errorf("clearing cache: %w", err)
			}
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

func shellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init",
		Short: "Emit shell integration functions",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(env.ShellIntegrationSnippet())
		},
	}
}

func unloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Emit unset commands for all tracked variables",
		RunE: func(cmd *cobra.Command, args []string) error {
			// In a real implementation, this would read the tracked env vars from a state file.
			// For now, emit a no-op.
			fmt.Println("# renv: no tracked variables to unload")
			return nil
		},
	}
}
