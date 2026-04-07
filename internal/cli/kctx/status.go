package kctx

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current KUBECONFIG, kubectl context, and loaded kubeconfigs",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := os.Stdout
			ui.Header(w, "Kubeconfig")

			kc := os.Getenv("KUBECONFIG")
			if kc == "" {
				ui.Item(w, "KUBECONFIG", ui.Gray(w, "not set"))
				ui.Item(w, "Managed by kctx", ui.Gray(w, "no"))
			} else {
				ui.Item(w, "KUBECONFIG", ui.Green(w, kc))
				managed := isManagedKubeconfig(kc)
				if managed {
					ui.Item(w, "Managed by kctx", ui.Green(w, "yes"))
				} else {
					ui.Item(w, "Managed by kctx", ui.Yellow(w, "no (external)"))
				}

				if ctx := currentKubectlContext(kc); ctx != "" {
					ui.Item(w, "Current context", ui.Bold(w, ctx))
				}
			}

			uid := fmt.Sprintf("%d", os.Getuid())
			store := kubeconfig.NewNamedStore()
			names, err := store.List(uid)
			if err != nil {
				slog.Warn("listing named kubeconfigs", "err", err)
			} else if len(names) > 0 {
				sort.Strings(names)
				ui.Header(w, "Loaded kubeconfigs (use 'kctx switch <name>')")
				ui.List(w, names)
			}
			return nil
		},
	}
}

func currentKubectlContext(kubeconfigPath string) string {
	cmd := exec.Command("kubectl", "config", "current-context")
	if kubeconfigPath != "" {
		env := make([]string, 0, len(os.Environ())-1)
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "KUBECONFIG=") {
				env = append(env, e)
			}
		}
		cmd.Env = append(env, "KUBECONFIG="+kubeconfigPath)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
