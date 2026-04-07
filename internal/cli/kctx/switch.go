package kctx

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func switchCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name> [bw://item|vault-path]",
		Short: "Switch to a named kubeconfig (or fetch one if a source is given)",
		Long: `Switch KUBECONFIG to a named kubeconfig previously loaded with 'kctx load'.

If the named kubeconfig is not in the local store, a source (bw:// or vault
path) may be provided to fetch it on the fly.

Examples:
  kctx switch prod                          # use pre-loaded 'prod'
  kctx switch staging bw://k8s/staging      # fetch directly if not pre-loaded`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			source := ""
			if len(args) > 1 {
				source = args[1]
			}
			slog.Debug("switching kubeconfig", "name", name, "source", source)

			uid := fmt.Sprintf("%d", os.Getuid())
			store := kubeconfig.NewNamedStore()

			var kubeconfigData []byte

			if err := kubeconfig.ValidateStoreName(name); err == nil {
				lp, lperr := bw.ReadLocalPassword()
				if lperr != nil {
					return lperr
				}
				data, err := store.Get(uid, name, lp)
				if err != nil {
					return fmt.Errorf("reading named kubeconfig %q: %w", name, err)
				}
				if data != nil {
					kubeconfigData = data
				}
			}

			if kubeconfigData == nil {
				if source == "" {
					return fmt.Errorf(
						"no pre-loaded kubeconfig named %q found\n"+
							"Run: kctx load %s <bw://item|vault-path>",
						name, name,
					)
				}
				var err error
				kubeconfigData, err = fetchKubeconfig(cfg, source)
				if err != nil {
					return fmt.Errorf("fetching kubeconfig for %q: %w", name, err)
				}
			}

			path, werr := kubeconfig.WriteKubeconfig(kubeconfigData)
			if werr != nil {
				return fmt.Errorf("writing kubeconfig: %w", werr)
			}

			fmt.Printf("export KUBECONFIG=%s\n", path)
			fmt.Printf("trap 'kctx unload' EXIT\n")

			srcLabel := resolveSourceLabel(name, source)
			panelEntries := []ui.PanelEntry{
				{Key: "KUBECONFIG", Value: path, Source: srcLabel},
			}
			if ctx := currentKubectlContext(path); ctx != "" {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: "Context", Value: ctx})
			}
			headline := fmt.Sprintf("Switched to %s", ui.Bold(os.Stderr, name))
			ui.Panel(os.Stderr, "kctx", headline, panelEntries, cfg.UI.Border)
			return nil
		},
	}
}

func fetchKubeconfig(cfg *config.Config, source string) ([]byte, error) {
	reg := newRegistry(cfg)
	uri := normalizeKubeconfigURI(source)
	val, err := reg.Resolve(uri)
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func resolveSourceLabel(name, source string) string {
	if source == "" {
		return "named store"
	}
	return source
}
