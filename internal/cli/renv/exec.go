package renv

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/spf13/cobra"
)

func execCmd(noCache *bool, cfg *config.Config) *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "exec -- command [args...]",
		Short: "Run a command with resolved env vars injected (no eval needed)",
		Long: `Resolve secret references from a .env file and execute a command with those
variables set in its environment. No eval required.

  renv exec -- myprogram --flag value
  renv exec --env secrets.env -- myprogram

The -- separator is required to distinguish renv flags from the command's args.
The resolved variables override any same-named variables already in the environment.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.Debug("running exec", "file", file, "command", args[0])
			reg := newRegistry(*noCache, cfg)

			entries, err := env.ResolveDotEnv(file, reg)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", file, err)
			}

			environ := os.Environ()
			for _, e := range entries {
				environ = append(environ, e.Key+"="+e.Value)
			}

			bin, err := exec.LookPath(args[0])
			if err != nil {
				return fmt.Errorf("%s: command not found", args[0])
			}

			return syscall.Exec(bin, args, environ)
		},
	}
	cmd.Flags().StringVarP(&file, "env", "e", ".env", "Path to .env file")
	return cmd
}
