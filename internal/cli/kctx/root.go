package kctx

import (
"log/slog"

"github.com/eficode/secure-handling-of-secrets/internal/config"
"github.com/eficode/secure-handling-of-secrets/internal/logger"
"github.com/eficode/secure-handling-of-secrets/internal/version"
"github.com/spf13/cobra"
)

// NewRootCmd returns the root cobra command for the kctx CLI.
func NewRootCmd() *cobra.Command {
var verbose bool
var cfgFile string
var logLevel string
var cfg config.Config

root := &cobra.Command{
Use:          "kctx",
Short:        "Ephemeral kubeconfig switcher via Vault or Bitwarden",
Version:      version.String(),
SilenceUsage: true,
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
var err error
cfg, err = config.Load(cfgFile)
if err != nil {
return err
}
if cmd.Root().PersistentFlags().Changed("log-level") {
cfg.Log.Level = logLevel
}
if verbose {
cfg.Log.Level = "debug"
}
logger.Init(cfg.Log.Level, cfg.Log.Format)
slog.Debug("config loaded",
"log_level", cfg.Log.Level,
"log_format", cfg.Log.Format,
"timeout_vault", cfg.Timeouts.Vault,
"timeout_bitwarden", cfg.Timeouts.Bitwarden,
)
return nil
},
}

root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

root.AddCommand(
loadCmd(&cfg),
switchCmd(&cfg),
unloadCmd(&cfg),
statusCmd(),
clearCacheCmd(),
shellInitCmd(),
watchCmd(),
)
return root
}

// NewSubCmd returns the kctx subcommand tree for embedding under another root (e.g. envoke).
// Unlike NewRootCmd, it does not register persistent flags (those are inherited from the parent)
// and does not install its own PersistentPreRunE (the parent's runs instead).
// cfg is a pointer owned by the parent, populated before any subcommand runs.
func NewSubCmd(cfg *config.Config) *cobra.Command {
root := &cobra.Command{
Use:   "kctx",
Short: "Ephemeral kubeconfig switcher via Vault or Bitwarden",
}
root.AddCommand(
loadCmd(cfg),
switchCmd(cfg),
unloadCmd(cfg),
statusCmd(),
clearCacheCmd(),
shellInitCmd(),
watchCmd(),
)
return root
}
