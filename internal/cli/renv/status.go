package renv

import (
	"fmt"
	"io"
	"os"
	"time"

	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/state"
	"github.com/eficode/secure-handling-of-secrets/internal/ui"
	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cache and loaded variable status",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := os.Stdout
			uid := fmt.Sprintf("%d", os.Getuid())

			ui.Header(w, "Tracked variables")
			names, err := state.LoadVarNames(uid)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				ui.Item(w, "Status", ui.Gray(w, "none loaded"))
			} else {
				ui.Item(w, "Status", ui.Green(w, fmt.Sprintf("%d loaded", len(names))))
				ui.List(w, names)
			}
			fmt.Fprintln(w)

			ui.Header(w, "Cache")
			cache := bw.NewCache()
			ui.Item(w, "Location", cache.Dir)
			files, ages, err := bw.CacheStatus(cache)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				ui.Item(w, "Files", ui.Gray(w, "none"))
			} else {
				for i, f := range files {
					age, err := time.ParseDuration(ages[i])
					ageStr := colorAge(w, ages[i], age, err)
					ui.Item(w, f, ageStr)
				}
			}

			return nil
		},
	}
}

func pluralVars(n int) string {
	if n == 1 {
		return "1 variable"
	}
	return fmt.Sprintf("%d variables", n)
}

func colorAge(w io.Writer, raw string, d time.Duration, parseErr error) string {
	if parseErr != nil {
		return raw
	}
	switch {
	case d < 30*time.Minute:
		return ui.Green(w, raw)
	case d < 4*time.Hour:
		return ui.Yellow(w, raw)
	default:
		return ui.Red(w, raw)
	}
}
