package kctx

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func unloadCmd(cfg *config.Config) *cobra.Command {
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

			if kubeconfigPath == "" {
				ui.Warn(os.Stderr, "KUBECONFIG was not set")
			} else {
				entries := []ui.PanelEntry{{Key: "Removed", Value: kubeconfigPath}}
				ui.Panel(os.Stderr, "kctx", "Kubeconfig unloaded", entries, cfg.UI.Border)
			}
			return nil
		},
	}
}

func isManagedKubeconfig(path string) bool {
	return kubeconfig.IsManaged(path)
}
