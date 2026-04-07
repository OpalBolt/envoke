package kctx

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/eficode/secure-handling-of-secrets/internal/cleanup"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/spf13/cobra"
)

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for sleep/lock events and manage kubeconfigs (run in background by shell-init)",
		Long: `Run in the background to manage kubeconfig tmpfiles and the secret cache
when the system sleeps or the screen is locked. Normally started automatically
by shell-init.

On lock: managed kubeconfig tmpfiles are removed and open shells are signalled
to unset KUBECONFIG. The encrypted cache is kept so configs can be quickly
re-resolved after unlock without re-entering passwords.

On sleep: the encrypted cache is also cleared, requiring full re-authentication
after wake.

On Linux, sleep and screen-lock events are detected via D-Bus (systemd-logind).
On macOS, sleep is detected via timer drift; screen lock requires a launchd agent.
On Windows, event hooks are not yet implemented.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			detachFromTerminal()
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("starting kctx watcher", "uid", uid)

			hook := cleanup.New()

			if err := hook.RegisterLock(func() error {
				slog.Debug("cleanup: clearing managed kubeconfigs on lock")
				kubeconfig.ClearManaged()
				_ = kubeconfig.RequestUnload(uid)
				return nil
			}); err != nil {
				return fmt.Errorf("registering lock hook: %w", err)
			}

			if err := hook.RegisterSleep(func() error {
				slog.Debug("cleanup: clearing kctx cache and managed kubeconfigs on sleep")
				cache := bw.NewCache()
				_ = cache.Clear(uid)
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
