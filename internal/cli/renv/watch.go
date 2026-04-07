package renv

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/eficode/secure-handling-of-secrets/internal/cleanup"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/state"
	"github.com/spf13/cobra"
)

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for sleep/lock events and manage secrets (run in background by shell-init)",
		Long: `Run in the background to manage secrets when the system sleeps or the screen
is locked. Normally started automatically by shell-init.

On lock: secret environment variables are unloaded from open shells. The
encrypted cache is kept so secrets can be quickly re-resolved after unlock
without re-entering passwords.

On sleep: the encrypted cache, stored session, and local passwords are cleared,
requiring full re-authentication after wake.

On Linux, sleep and screen-lock events are detected via D-Bus (systemd-logind).
On macOS, sleep is detected via timer drift; screen lock requires a launchd agent.
On Windows, event hooks are not yet implemented.

Start manually:
  renv watch &`,
		RunE: func(cmd *cobra.Command, args []string) error {
			detachFromTerminal()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("starting renv watcher", "uid", uid)

			hook := cleanup.New()

			if err := hook.RegisterLock(func() error {
				slog.Debug("cleanup: unloading renv variables on lock")
				_ = state.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing renv cache and session on sleep")
				cache := bw.NewCache()
				_ = cache.Clear(uid)
				_ = bw.ClearStoredSession(uid)
				_ = bw.ClearStoredLocalPassword(uid)
				_ = state.RequestUnload(uid)
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
