package ui

import (
	"os"
	"testing"
	"time"
)

// ExampleSpinner demonstrates basic spinner usage.
// This shows how to display progress while an operation runs.
func ExampleSpinner() {
	spinner := NewSpinner(os.Stderr, "Loading secrets from Bitwarden...")
	spinner.Start()

	// Simulate work
	time.Sleep(2 * time.Second)

	// Update the message
	spinner.SetMessage("Processing items...")
	time.Sleep(1 * time.Second)

	// Stop the spinner
	spinner.Stop()
}

// TestSpinnerBasic verifies the spinner can be created and started/stopped.
func TestSpinnerBasic(t *testing.T) {
	spinner := NewSpinner(os.Stderr, "Testing spinner")
	spinner.Start()
	time.Sleep(200 * time.Millisecond)
	spinner.Stop()
	// If we get here without panic, the test passes
}

// TestSpinnerSetMessage verifies the message can be updated.
func TestSpinnerSetMessage(t *testing.T) {
	spinner := NewSpinner(os.Stderr, "Initial message")
	spinner.Start()
	spinner.SetMessage("Updated message")
	time.Sleep(100 * time.Millisecond)
	spinner.Stop()
}

// TestProgressTracker verifies the progress tracker works.
func TestProgressTracker(t *testing.T) {
	tracker := NewProgressTracker(os.Stderr, "Operations")
	tracker.AddStep("Connecting to server")
	tracker.AddStep("Fetching data")
	tracker.AddStep("Processing results")

	tracker.StartStep(0)
	time.Sleep(100 * time.Millisecond)
	tracker.CompleteStep()

	tracker.StartStep(1)
	time.Sleep(100 * time.Millisecond)
	tracker.CompleteStep()

	tracker.StartStep(2)
	time.Sleep(100 * time.Millisecond)
	tracker.CompleteStep()
}
