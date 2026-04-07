package renv

import (
"log/slog"

"github.com/eficode/secure-handling-of-secrets/internal/config"
"github.com/eficode/secure-handling-of-secrets/internal/logger"
"github.com/eficode/secure-handling-of-secrets/internal/version"
"github.com/spf13/cobra"
)

// NewRootCmd returns the root cobra command for the renv CLI.
func NewRootCmd() *cobra.Command {
var verbose bool
var noCache bool
var cfgFile string
var logLevel string
var cfg config.Config

root := &cobra.Command{
Use:          "renv",
Short:        "Resolve secret references in .env and YAML files",
Version:      version.String(),
SilenceUsage: true,
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
var err error
cfg, err = config.Load(cfgFile)
if err != nil {
return err
}
// CLI flags override config/env
if cmd.Flags().Changed("log-level") || cmd.Root().PersistentFlags().Changed("log-level") {
cfg.Log.Level = logLevel
}
if verbose {
cfg.Log.Level = "debug"
}
logger.Init(cfg.Log.Level, cfg.Log.Format)
slog.Debug("config loaded",
"log_level", cfg.Log.Level,
"log_format", cfg.Log.Format,
"cache_max_age", cfg.Cache.MaxAge,
"timeout_bitwarden", cfg.Timeouts.Bitwarden,
"timeout_vault", cfg.Timeouts.Vault,
)
return nil
},
}

root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
root.PersistentFlags().BoolVar(&noCache, "no-cache", false, "Disable encrypted cache")
root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

// Deprecated flags kept for backwards compatibility — they are now no-ops.
var ignoredBool bool
var ignoredString string
root.PersistentFlags().BoolVar(&ignoredBool, "isolated", false, "Deprecated: no longer has any effect")
root.PersistentFlags().StringVar(&ignoredString, "password-grace-period", "", "Deprecated: no longer has any effect")
_ = root.PersistentFlags().MarkDeprecated("isolated", "this flag no longer has any effect and will be removed in a future release")
_ = root.PersistentFlags().MarkDeprecated("password-grace-period", "this flag no longer has any effect and will be removed in a future release")

root.AddCommand(
resolveCmd(&noCache, &cfg),
execCmd(&noCache, &cfg),
shellInitCmd(),
yamlCmd(&cfg),
clearCacheCmd(),
statusCmd(),
unloadCmd(&cfg),
watchCmd(),
)
return root
}

// NewSubCmd returns the renv subcommand tree for embedding under another root (e.g. envoke).
// Unlike NewRootCmd, it does not register persistent flags (those are inherited from the parent)
// and does not install its own PersistentPreRunE (the parent's runs instead).
// noCache and cfg are pointers owned by the parent, populated before any subcommand runs.
func NewSubCmd(noCache *bool, cfg *config.Config) *cobra.Command {
root := &cobra.Command{
Use:   "renv",
Short: "Resolve secret references in .env and YAML files",
}
root.AddCommand(
resolveCmd(noCache, cfg),
execCmd(noCache, cfg),
shellInitCmd(),
yamlCmd(cfg),
clearCacheCmd(),
statusCmd(),
unloadCmd(cfg),
watchCmd(),
)
return root
}
