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
	"golang.org/x/term"
)

func resolveCmd(noCache *bool, cfg *config.Config) *cobra.Command {
	var file string
	var shell string

	cmd := &cobra.Command{
		Use:   "resolve [file]",
		Short: "Resolve .env file secret references and emit shell exports",
		Long: `Resolve secret references in a .env file and print shell export statements.

The output must be evaluated by your shell to set the variables:

  eval "$(renv resolve .env)"

With direnv, use a use_renv helper so direnv fully owns the load/unload lifecycle.
Add to ~/.config/direnv/direnvrc:

  use_renv() {
    local file="${1:-.env}"
    watch_file "$file"
    eval "$(renv unload 2>/dev/null || true)"
    eval "$(renv resolve "$file")"
  }

Then in .envrc:

  use renv .env`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				file = args[0]
			}
			slog.Debug("running resolve", "file", file, "shell", shell)
			if term.IsTerminal(int(os.Stdout.Fd())) {
				ui.Warn(os.Stderr, "stdout is a terminal — output will not be set as env vars.")
				fmt.Fprintln(os.Stderr, "  use: eval \"$(renv resolve .env)\"")
			}
			reg := newRegistry(*noCache, cfg)

			entries, err := env.ResolveDotEnv(file, reg)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", file, err)
			}

			if err := env.EmitExports(os.Stdout, entries); err != nil {
				return err
			}

			uid := fmt.Sprintf("%d", os.Getuid())
			names := make([]string, len(entries))
			panelEntries := make([]ui.PanelEntry, len(entries))
			for i, e := range entries {
				names[i] = e.Key
				panelEntries[i] = ui.PanelEntry{Key: e.Key, Source: e.Source}
			}
			_ = state.SaveVarNames(uid, names)

			headline := fmt.Sprintf("Loaded %s from %s",
				ui.Bold(os.Stderr, pluralVars(len(entries))),
				ui.Bold(os.Stderr, file))
			ui.Panel(os.Stderr, "renv", headline, panelEntries, cfg.UI.Border)

			inManagedEnv := os.Getenv("DIRENV_DIR") != "" ||
				os.Getenv("DIRENV_FILE") != "" ||
				os.Getenv("IN_NIX_SHELL") != ""
			if !inManagedEnv {
				switch shell {
				case "fish":
					fmt.Println("# Fish shell trap not supported via eval; use renv clear-cache manually")
				default:
					fmt.Println("trap 'renv clear-cache' EXIT")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", ".env", "Path to .env file")
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type (bash|fish|zsh)")
	return cmd
}
