package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/logger"
	"github.com/eficode/secure-handling-of-secrets/internal/secrets"
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

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current KUBECONFIG and kubectl context",
		RunE: func(cmd *cobra.Command, args []string) error {
			kc := os.Getenv("KUBECONFIG")
			if kc == "" {
				fmt.Println("KUBECONFIG: (not set)")
			} else {
				fmt.Printf("KUBECONFIG: %s\n", kc)
			}
			return nil
		},
	}
}

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove all kctx cache files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := secrets.NewCache()
			uid := fmt.Sprintf("%d", os.Getuid())
			return cache.Clear(uid)
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
# kctx shell integration — source this into your shell
# Usage: source <(kctx shell-init)

kctx() {
  case "$1" in
    unload)
      eval "$(command kctx unload)"
      ;;
    status)
      command kctx status
      ;;
    *)
      eval "$(command kctx switch "$@")"
      ;;
  esac
}
`
}
