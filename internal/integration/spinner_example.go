// Package integration provides examples of how to use the spinner and progress features.
// This shows best practices for integrating progress feedback into renv and kctx commands.
package integration

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/opalbolt/envoke/internal/ui"
)

// ExampleResolveWithProgress demonstrates how to add spinner feedback
// to the resolve command when loading secrets.
//
// This would be integrated into the renv resolve command as follows:
//
//	spinner := ui.NewSpinner(os.Stderr, "Connecting to Bitwarden...")
//	spinner.Start()
//	defer spinner.Stop()
//
//	// First Bitwarden access will unlock the session
//	entries, err := env.ResolveDotEnv(file, reg)
//	if err != nil {
//		return err
//	}
//
//	// Continue with normal flow...
//
func ExampleResolveWithProgress(w io.Writer) error {
	slog.Info("demonstrating spinner usage")

	// Example 1: Simple spinner during a long operation
	spinner := ui.NewSpinner(w, "Loading configuration...")
	spinner.Start()
	time.Sleep(1 * time.Second)

	spinner.SetMessage("Connecting to Bitwarden...")
	time.Sleep(500 * time.Millisecond)

	spinner.SetMessage("Unlocking vault...")
	time.Sleep(500 * time.Millisecond)

	spinner.SetMessage("Fetching secrets...")
	time.Sleep(500 * time.Millisecond)

	spinner.Stop()

	return nil
}

// ExampleProgressTrackerUsage demonstrates the ProgressTracker for multi-step operations.
//
// This would be useful for commands like:
// - renv exec (with progress through each secret resolution step)
// - kctx switch (with progress through kubeconfig setup steps)
//
func ExampleProgressTrackerUsage(w io.Writer) error {
	tracker := ui.NewProgressTracker(w, "Secret Resolution")

	// Define the steps
	tracker.AddStep("Connecting to Bitwarden")
	tracker.AddStep("Fetching folder list")
	tracker.AddStep("Resolving secrets")
	tracker.AddStep("Caching results")

	// Execute each step
	steps := []struct {
		name     string
		duration time.Duration
	}{
		{"Connecting to Bitwarden", 500 * time.Millisecond},
		{"Fetching folder list", 800 * time.Millisecond},
		{"Resolving secrets", 1200 * time.Millisecond},
		{"Caching results", 300 * time.Millisecond},
	}

	for i := range steps {
		tracker.StartStep(i)
		time.Sleep(steps[i].duration)
		tracker.CompleteStep()
	}

	return nil
}

// ExampleProgressRegistryUsage shows how to wrap a registry for automatic
// spinner feedback during secret resolution.
//
// Usage in renv:
//
//	baseReg := newRegistry(cfg)
//	progressReg := ui.NewProgressRegistry(baseReg, os.Stderr)
//	defer progressReg.Close()
//
//	entries, err := env.ResolveDotEnvWithRegistry(file, progressReg)
//	// Spinner runs automatically during resolution
//
func ExampleProgressRegistryUsage() {
	fmt.Println(`
// In internal/cli/renv/root.go, the resolve command would use:

func resolveCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			reg := newRegistry(cfg)

			// Wrap with progress feedback
			progressReg := ui.NewProgressRegistry(reg, os.Stderr)
			defer progressReg.Close()

			// During resolution, a spinner will show progress
			entries, err := env.ResolveDotEnv(file, progressReg)
			if err != nil {
				return fmt.Errorf("resolving %s: %w", file, err)
			}

			// Rest of the resolve logic...
			return nil
		},
	}
	return cmd
}
	`)
}
