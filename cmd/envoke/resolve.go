package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/providers"
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
		Short: "Resolve .env secrets and kubeconfig directives",
		Long: `Resolve a .env file, handling both secret references and kubeconfig directives.

Secret references (bw://, vault://) are resolved and exported as shell variables.

Kubeconfig directives (keys prefixed with KCTX_) load a kubeconfig into the
local kctx named store without exporting the raw value:

  KCTX_PROD=bw://kubernetes/prod-cluster     # loads kubeconfig named "prod"
  KCTX_STAGING=vault://secret/kubeconfigs    # loads kubeconfig named "staging"

After envoke resolve, switch to a loaded kubeconfig with:

  kctx prod       (shorthand for: kctx switch prod)

The output must be evaluated by your shell:

  eval "$(envoke resolve .env)"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				file = args[0]
			}
			slog.Debug("running envoke resolve", "file", file)
			if term.IsTerminal(int(os.Stdout.Fd())) {
				ui.Warn(os.Stderr, "stdout is a terminal — output will not be set as env vars.")
				fmt.Fprintln(os.Stderr, "  use: eval \"$(envoke resolve .env)\"")
			}

			// Parse the raw .env file to separate kctx directives from env secrets.
			rawEntries, err := env.ParseRaw(file)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", file, err)
			}

			var kctxEntries []env.RawEntry
			var envEntries []env.RawEntry
			for _, e := range rawEntries {
				if isKctxDirective(e) {
					kctxEntries = append(kctxEntries, e)
				} else {
					envEntries = append(envEntries, e)
				}
			}

			// Pre-flight: validate all KCTX names before doing any secret fetching.
			for _, e := range kctxEntries {
				if err := kubeconfig.ValidateStoreName(kctxNameFromKey(e.Key)); err != nil {
					return fmt.Errorf("invalid kctx directive %q: %w", e.Key, err)
				}
			}

			// Create ONE shared registry for the entire resolve operation.
			sharedReg := newRegistry(*noCache, cfg)

			// Handle kubeconfig directives using the shared registry.
			var kctxPanelEntries []ui.PanelEntry
			if len(kctxEntries) > 0 {
				uid := fmt.Sprintf("%d", os.Getuid())
				store := kubeconfig.NewNamedStore()
				var kctxNames []string
				for _, e := range kctxEntries {
					name := kctxNameFromKey(e.Key)
					if err := fetchKubeconfigForDirective(sharedReg, name, e.Value, uid, store); err != nil {
						if errors.Is(err, bw.ErrInvalidPassword) {
							return err
						}
						return fmt.Errorf("loading kubeconfig %q (%s): %w", name, e.Value, err)
					}
					kctxNames = append(kctxNames, name)
					kctxPanelEntries = append(kctxPanelEntries, ui.PanelEntry{
						Key:    name,
						Source: e.Value,
					})
					slog.Debug("loaded kubeconfig into kctx store", "name", name, "source", e.Value)
				}
				_ = kubeconfig.SaveTrackedNames(uid, kctxNames)
			}

			// Handle env secret entries using the same shared registry.
			var resolvedEntries []env.EnvEntry
			if len(envEntries) > 0 {
				tmpFile, err := writeTempEnv(envEntries)
				if err != nil {
					return fmt.Errorf("preparing env entries: %w", err)
				}
				defer os.Remove(tmpFile)

				resolvedEntries, err = env.ResolveDotEnv(tmpFile, sharedReg)
				if err != nil {
					if errors.Is(err, bw.ErrInvalidPassword) {
						return err
					}
					return fmt.Errorf("resolving %s: %w", file, err)
				}
			}

			if err := env.EmitExports(os.Stdout, resolvedEntries); err != nil {
				return err
			}

			if len(resolvedEntries) > 0 {
				uid := fmt.Sprintf("%d", os.Getuid())
				names := make([]string, len(resolvedEntries))
				for i, e := range resolvedEntries {
					names[i] = e.Key
				}
				_ = state.SaveVarNames(uid, names)
			}

			panelEntries := make([]ui.PanelEntry, 0, len(resolvedEntries)+len(kctxPanelEntries))
			for _, e := range resolvedEntries {
				panelEntries = append(panelEntries, ui.PanelEntry{Key: e.Key, Source: e.Source})
			}
			for _, e := range kctxPanelEntries {
				panelEntries = append(panelEntries, ui.PanelEntry{
					Key:    "kctx:" + e.Key,
					Source: e.Source,
				})
			}
			totalCount := len(resolvedEntries) + len(kctxPanelEntries)
			headline := fmt.Sprintf("Loaded %s from %s",
				ui.Bold(os.Stderr, pluralItems(totalCount)),
				ui.Bold(os.Stderr, file))
			ui.Panel(os.Stderr, "envoke", headline, panelEntries, cfg.UI.Border)

			inManagedEnv := os.Getenv("DIRENV_DIR") != "" ||
				os.Getenv("DIRENV_FILE") != "" ||
				os.Getenv("IN_NIX_SHELL") != ""
			if !inManagedEnv {
				switch shell {
				case "fish":
					fmt.Println("# Fish shell trap not supported via eval; use envoke clear-cache manually")
				default:
					fmt.Println("trap 'envoke clear-cache' EXIT")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", ".env", "Path to .env file")
	cmd.Flags().StringVar(&shell, "shell", "bash", "Shell type (bash|fish|zsh)")
	return cmd
}

// isKctxDirective returns true if the entry is a KCTX_<name>=bw:// or vault:// directive.
func isKctxDirective(e env.RawEntry) bool {
	if !strings.HasPrefix(e.Key, "KCTX_") {
		return false
	}
	return strings.HasPrefix(e.Value, "bw://") || strings.HasPrefix(e.Value, "vault://")
}

// kctxNameFromKey derives a kubeconfig store name from a KCTX_ key.
// KCTX_PROD → "prod", KCTX_MY_CLUSTER → "my_cluster"
func kctxNameFromKey(key string) string {
	return strings.ToLower(strings.TrimPrefix(key, "KCTX_"))
}

// fetchKubeconfigForDirective fetches kubeconfig bytes from a bw:// or vault:// source,
// stores them in the named store using the registry's shared local password.
func fetchKubeconfigForDirective(reg *providers.Registry, name, source, uid string, store *kubeconfig.NamedStore) error {
	uri := normalizeKubeconfigURI(source)
	val, err := reg.Resolve(uri)
	if err != nil {
		return err
	}
	kubeconfigData := []byte(val)

	localPassword := reg.LocalPassword()
	if localPassword == "" {
		lp, err := bw.ReadLocalPassword()
		if err != nil {
			return err
		}
		localPassword = lp
	}

	return store.Put(uid, name, localPassword, kubeconfigData)
}

// writeTempEnv writes env entries to a temp .env file for processing by ResolveDotEnv.
func writeTempEnv(entries []env.RawEntry) (string, error) {
	f, err := os.CreateTemp("", "envoke-*.env")
	if err != nil {
		return "", err
	}
	defer f.Close()
	for _, e := range entries {
		if _, err := fmt.Fprintf(f, "%s=%s\n", e.Key, e.Value); err != nil {
			return "", err
		}
	}
	return f.Name(), nil
}

func pluralItems(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
