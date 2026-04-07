package kctx

import (
	"fmt"
	"log/slog"
	"os"

	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove all kctx cache files and named kubeconfigs",
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("clearing kctx cache", "uid", uid)

			cache := bw.NewCache()
			if err := cache.Clear(uid); err != nil {
				return err
			}

			store := kubeconfig.NewNamedStore()
			if err := store.Clear(uid); err != nil {
				return err
			}

			ui.Success(os.Stderr, "Cache cleared")
			return nil
		},
	}
}
