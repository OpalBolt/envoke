package main

import (
	"log/slog"
	"os"

	intkctx "github.com/eficode/secure-handling-of-secrets/internal/cli/kctx"
	intrenv "github.com/eficode/secure-handling-of-secrets/internal/cli/renv"
	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/logger"
	"github.com/eficode/secure-handling-of-secrets/internal/providers"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	vlt "github.com/eficode/secure-handling-of-secrets/internal/providers/vault"
	"github.com/eficode/secure-handling-of-secrets/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var verbose bool
	var noCache bool
	var cfgFile string
	var logLevel string
	var cfg config.Config

	root := &cobra.Command{
		Use:   "envoke",
		Short: "Unified secret environment loader — env vars and kubeconfigs",
		Long: `envoke (env + invoke) is a single binary combining renv and kctx.

  envoke resolve .env            # resolve both env secrets and kubeconfig refs
  envoke renv resolve .env       # renv subcommands (env secret resolution)
  envoke kctx switch prod        # kctx subcommands (kubeconfig switching)
  envoke shell-init              # combined shell setup for both renv and kctx

The .env file supports KCTX_<name>=bw://... or KCTX_<name>=vault://...
entries that load kubeconfigs into the local kctx named store.`,
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
			)
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable debug logging (shorthand for --log-level=debug)")
	root.PersistentFlags().BoolVar(&noCache, "no-cache", false, "Disable encrypted cache")
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "Config file path (default: $XDG_CONFIG_HOME/renv/config.yaml)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error")
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	// Embed renv and kctx command trees as subcommands.
	// Use NewSubCmd (not NewRootCmd) so the sub-trees do not re-register persistent
	// flags that are already defined on the envoke root — which would cause collisions
	// via Cobra's inherited persistent-flag merging. cfg and noCache are populated by
	// envoke's own PersistentPreRunE before any subcommand's RunE executes.
	renvRoot := intrenv.NewSubCmd(&noCache, &cfg)
	kctxRoot := intkctx.NewSubCmd(&cfg)

	root.AddCommand(
		renvRoot,
		kctxRoot,
		resolveCmd(&noCache, &cfg),
		unloadCmd(&cfg),
		shellInitCmd(),
		clearCacheCmd(),
		watchCmd(),
	)
	return root
}

// newRegistry builds a secrets Registry with BW and Vault providers.
// noCache disables the disk cache; cfg supplies timeouts and cache settings.
// Use a single registry across the entire resolve operation so that LocalPassword
// and the BW session are shared and users are prompted at most once per invocation.
func newRegistry(noCache bool, cfg *config.Config) *providers.Registry {
	cache := bw.NewCache()
	cache.MaxAge = cfg.CacheMaxAge()
	if noCache {
		cache.Disabled = true
	}
	bwClient := &bw.BWClient{
		Cache:   cache,
		Timeout: cfg.BitwardenTimeout(),
	}
	vaultClient := &vlt.VaultClient{Timeout: cfg.VaultTimeout()}

	reg := providers.NewRegistry()
	reg.Register(providers.NewBWProvider(bwClient))
	reg.Register(providers.NewVaultProvider(vaultClient))
	return reg
}

// normalizeKubeconfigURI normalises a kubeconfig source URI.
// Delegates to kubeconfig.NormalizeSourceURI.
func normalizeKubeconfigURI(source string) string {
	return kubeconfig.NormalizeSourceURI(source)
}
