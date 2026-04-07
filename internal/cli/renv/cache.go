package renv

import (
	"fmt"
	"log/slog"
	"os"

	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove all renv cache files and stored session",
		Long: `Remove the encrypted secret cache and stored Bitwarden session.

Variable name tracking (used by renv unload) is intentionally preserved so that
renv unload continues to work after a cache clear — for example when the EXIT
trap fires inside a direnv subprocess.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := bw.NewCache()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("clearing cache and session", "uid", uid)
			if err := cache.Clear(uid); err != nil {
				return fmt.Errorf("clearing cache: %w", err)
			}
			if err := bw.ClearStoredSession(uid); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}
			if err := bw.ClearStoredLocalPassword(uid); err != nil {
				return fmt.Errorf("clearing local password: %w", err)
			}
			ui.Success(os.Stderr, "Cache cleared")
			return nil
		},
	}
}
