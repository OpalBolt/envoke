package kctx

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func loadCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "load <name> <bw://item|vault-path>",
		Short: "Fetch a kubeconfig and cache it under a local name",
		Long: `Fetch a kubeconfig from Bitwarden or Vault and encrypt it in the local
named store so that 'kctx switch <name>' can load it without re-fetching.

Place multiple kctx load calls in your .env file to pre-load all configs.
Both the local password and the Bitwarden password are prompted fresh on
every call — no passwords are persisted or shared between invocations.

Examples:
  kctx load prod bw://kubernetes/prod
  kctx load staging vault://secret/kubeconfig/staging`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			source := args[1]

			if err := kubeconfig.ValidateStoreName(name); err != nil {
				return err
			}

			slog.Debug("loading kubeconfig", "name", name, "source", source)

			uid := fmt.Sprintf("%d", os.Getuid())
			store := kubeconfig.NewNamedStore()

			reg := newRegistry(cfg)
			uri := normalizeKubeconfigURI(source)

			val, err := reg.Resolve(uri)
			if err != nil {
				return err
			}
			kubeconfigData := []byte(val)

			localPassword, err := kctxLocalPassword(reg)
			if err != nil {
				return err
			}

			if err := store.Put(uid, name, localPassword, kubeconfigData); err != nil {
				return fmt.Errorf("storing kubeconfig %q: %w", name, err)
			}

			ui.Success(os.Stderr, fmt.Sprintf("Loaded kubeconfig: %s", ui.Bold(os.Stderr, name)))
			return nil
		},
	}
}
