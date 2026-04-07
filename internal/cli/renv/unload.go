package renv

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/eficode/secure-handling-of-secrets/internal/state"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func unloadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "unload",
		Short: "Emit unset commands for all tracked variables",
		Long: `Emit shell unset commands for all variables exported by renv resolve.

The output must be evaluated by your shell:

  eval "$(renv unload)"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			uid := fmt.Sprintf("%d", os.Getuid())
			slog.Debug("unloading tracked variables", "uid", uid)
			names, err := state.LoadVarNames(uid)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				ui.Warn(os.Stderr, "No tracked variables to unload")
				return nil
			}
			entries := make([]env.EnvEntry, len(names))
			for i, name := range names {
				entries[i] = env.EnvEntry{Key: name}
			}
			if err := env.EmitUnload(os.Stdout, entries); err != nil {
				return err
			}
			_ = state.ClearVarNames(uid)

			panelEntries := make([]ui.PanelEntry, len(names))
			for i, n := range names {
				panelEntries[i] = ui.PanelEntry{Key: n}
			}
			headline := fmt.Sprintf("Unloaded %s", ui.Bold(os.Stderr, pluralVars(len(names))))
			ui.Panel(os.Stderr, "renv", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
}
