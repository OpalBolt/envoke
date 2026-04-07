package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/eficode/secure-handling-of-secrets/internal/cleanup"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/state"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func clearCacheCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-cache",
		Short: "Remove all envoke cache files (renv + kctx)",
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("clearing all caches", "uid", uid)

			cache := bw.NewCache()
			if err := cache.Clear(uid); err != nil {
				return fmt.Errorf("clearing secret cache: %w", err)
			}
			if err := bw.ClearStoredSession(uid); err != nil {
				return fmt.Errorf("clearing session: %w", err)
			}
			if err := bw.ClearStoredLocalPassword(uid); err != nil {
				return fmt.Errorf("clearing local password: %w", err)
			}

			store := kubeconfig.NewNamedStore()
			if err := store.Clear(uid); err != nil {
				return fmt.Errorf("clearing kubeconfig store: %w", err)
			}

			ui.Success(os.Stderr, "All caches cleared")
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
				slog.Debug("cleanup: unloading renv variables and kctx kubeconfigs on lock")
				_ = state.RequestUnload(uid)
				kubeconfig.ClearManaged()
				_ = kubeconfig.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing all caches on sleep")
				cache := bw.NewCache()
				_ = cache.Clear(uid)
				_ = bw.ClearStoredSession(uid)
				_ = bw.ClearStoredLocalPassword(uid)
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
