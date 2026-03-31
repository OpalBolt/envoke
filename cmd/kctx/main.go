package main

import (
	"fmt"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
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
	root := &cobra.Command{
		Use:   "kctx-bin",
		Short: "Ephemeral kubeconfig switcher via Vault or Bitwarden",
	}
	root.AddCommand(
		switchCmd(),
		clearCmd(),
		statusCmd(),
		cacheClearCmd(),
		versionCmd(),
		shellInitCmd(),
	)
	return root
}

func switchCmd() *cobra.Command {
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

			var kubeconfigData []byte

			if source == "" || source == env {
				// Default: try vault path based on env name
				vaultRef := secrets.VaultRef{Path: "secret/kubeconfig/" + env, Field: "kubeconfig"}
				vc := &secrets.VaultClient{}
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
				bwClient := &secrets.BWClient{Cache: cache}
				val, bwerr := bwClient.Resolve(ref)
				if bwerr != nil {
					return bwerr
				}
				kubeconfigData = []byte(val)
			} else {
				// Treat as vault path
				vaultRef := secrets.VaultRef{Path: source, Field: "kubeconfig"}
				vc := &secrets.VaultClient{}
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
			fmt.Printf("trap 'kctx-bin clear' EXIT\n")
			return nil
		},
	}
}

func clearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Unset KUBECONFIG and remove tmpfile",
		RunE: func(cmd *cobra.Command, args []string) error {
			kubeconfigPath := os.Getenv("KUBECONFIG")
			if kubeconfigPath != "" {
				_ = os.Remove(kubeconfigPath)
			}
			fmt.Println("unset KUBECONFIG")
			return nil
		},
	}
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

func cacheClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cache-clear",
		Short: "Remove all kctx cache files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := secrets.NewCache()
			uid := fmt.Sprintf("%d", os.Getuid())
			return cache.Clear(uid)
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kctx %s\n", version)
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
# Usage: source <(kctx-bin shell-init)

kctx() {
  case "$1" in
    clear)
      eval "$(kctx-bin clear)"
      ;;
    status)
      kctx-bin status
      ;;
    *)
      eval "$(kctx-bin switch "$@")"
      ;;
  esac
}
`
}
