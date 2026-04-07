package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/state"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func unloadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Unset all tracked env vars and KUBECONFIG",
		Long: `Emit shell commands to unset all variables exported by envoke resolve,
and unset KUBECONFIG if it was set by kctx.

The output must be evaluated by your shell:

  eval "$(envoke unload)"

When using the envoke shell-init, the shell function handles this automatically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("unloading all envoke state", "uid", uid)

			var panelEntries []ui.PanelEntry

			names, err := state.LoadVarNames(uid)
			if err != nil {
				slog.Warn("loading tracked var names", "err", err)
			}
			if len(names) > 0 {
				entries := make([]env.EnvEntry, len(names))
				for i, name := range names {
					entries[i] = env.EnvEntry{Key: name}
				}
				if emitErr := env.EmitUnload(os.Stdout, entries); emitErr != nil {
					slog.Warn("emitting unload", "err", emitErr)
				} else {
					_ = state.ClearVarNames(uid)
					for _, n := range names {
						panelEntries = append(panelEntries, ui.PanelEntry{Key: n})
					}
				}
			}

			kubeconfigPath := os.Getenv("KUBECONFIG")
			if kubeconfigPath != "" && kubeconfig.IsManaged(kubeconfigPath) {
				if err := os.Remove(kubeconfigPath); err != nil && !os.IsNotExist(err) {
					slog.Warn("removing managed kubeconfig", "path", kubeconfigPath, "err", err)
				}
				panelEntries = append(panelEntries, ui.PanelEntry{
					Key:   "KUBECONFIG",
					Value: kubeconfigPath,
				})
			}
			fmt.Fprintln(os.Stdout, "unset KUBECONFIG")

			kctxNames, err := kubeconfig.LoadTrackedNames(uid)
			if err != nil {
				slog.Warn("loading tracked kctx names", "err", err)
			}
			if len(kctxNames) > 0 {
				store := kubeconfig.NewNamedStore()
				for _, name := range kctxNames {
					if rmErr := store.Remove(uid, name); rmErr != nil {
						slog.Warn("removing kctx store entry", "name", name, "err", rmErr)
					} else {
						slog.Debug("removed kctx store entry", "name", name)
						panelEntries = append(panelEntries, ui.PanelEntry{
							Key:   "kctx:" + name,
							Value: "(store)",
						})
					}
				}
				_ = kubeconfig.ClearTrackedNames(uid)
			}

			if len(panelEntries) == 0 {
				ui.Warn(os.Stderr, "Nothing to unload")
				return nil
			}
			headline := fmt.Sprintf("Unloaded %s", ui.Bold(os.Stderr, pluralItems(len(panelEntries))))
			ui.Panel(os.Stderr, "envoke", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
}
