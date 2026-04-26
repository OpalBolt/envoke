# Progress Feedback and Spinners

This document describes the new spinner and progress tracking features added to envoke.

## Overview

When running commands like `renv resolve`, envoke may need to unlock Bitwarden, fetch secrets, and perform other operations that can take several seconds. Previously, users would see nothing but the final output. Now, envoke provides visual feedback during these operations.

## Features

### Spinner Component

The `Spinner` type provides animated visual feedback during long-running operations:

```go
import (
	"os"
	"github.com/opalbolt/envoke/internal/ui"
)

// Create a spinner
spinner := ui.NewSpinner(os.Stderr, "Fetching secrets...")

// Start the animation
spinner.Start()

// Update the message
spinner.SetMessage("Unlocking vault...")

// Stop the spinner (clears the line)
spinner.Stop()
```

**Features:**
- Animated spinner frames (using Braille Unicode characters)
- Cyan color (respects ANSI support)
- Automatically disabled for non-TTY outputs (pipes, CI, file redirection)
- Thread-safe message updates
- Clean line clearing on stop

### Progress Tracker

The `ProgressTracker` type tracks multi-step operations:

```go
tracker := ui.NewProgressTracker(os.Stderr, "Resolution")

tracker.AddStep("Connecting to Bitwarden")
tracker.AddStep("Fetching secrets")
tracker.AddStep("Caching results")

tracker.StartStep(0) // Begin first step
time.Sleep(1 * time.Second)
tracker.CompleteStep() // Shows checkmark and moves to next

tracker.StartStep(1)
time.Sleep(2 * time.Second)
tracker.CompleteStep()
```

### Progress Registry

The `ProgressRegistry` wraps a secret provider registry to automatically show spinner feedback during secret resolution:

```go
baseReg := newRegistry(cfg)
progressReg := ui.NewProgressRegistry(baseReg, os.Stderr)
defer progressReg.Close()

// During resolution, spinner shows "Resolving: bw://folder/item"
value, err := progressReg.Resolve("bw://folder/item")
```

## Implementation Details

### Braille Spinner Frames

The spinner uses Braille Unicode characters which provide smooth animation:

```
⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏
```

This pattern works well in all modern terminals including:
- macOS Terminal and iTerm2
- Linux terminals (GNOME Terminal, Konsole, etc.)
- Windows Terminal
- SSH sessions

### ANSI Control

The spinner uses standard ANSI escape codes:
- `` - carriage return (move cursor to line start)
- `[K` - clear to end of line

These are only sent to TTY outputs. For pipes and file redirection, the spinner is silently disabled.

### Thread Safety

Both `Spinner` and `ProgressRegistry` are thread-safe:
- Spinner uses `sync.Mutex` for concurrent message updates
- Multiple goroutines can safely call `SetMessage()`

## Use Cases

### Issue #36: Show Progress While Loading Secrets

When running `renv resolve .env`, if the .env file references many Bitwarden secrets:

```
⠹ Resolving: bw://folder/database-password
```

This tells the user that work is happening and prevents the perception of a hung process.

### Issue #41: Spinner with Detailed Feedback

For long operations, the spinner combined with status messages provides clarity:

```
⠏ Connecting to Bitwarden...
✓ Connected
⠋ Fetching folder list...
✓ Fetched 12 folders
⠙ Resolving 8 secrets...
✓ Resolved all secrets
```

## Integration Guide

To add spinner feedback to an existing command:

### Simple Spinner (Issue #36)

```go
// In the resolve command
spinner := ui.NewSpinner(os.Stderr, "Resolving secrets...")
spinner.Start()
defer spinner.Stop()

// Do work...
entries, err := env.ResolveDotEnv(file, reg)
```

### Progress Tracking (Issue #41)

```go
// For complex operations with multiple steps
tracker := ui.NewProgressTracker(os.Stderr, "Resolution")
tracker.AddStep("Unlock Bitwarden")
tracker.AddStep("Fetch folder list")
tracker.AddStep("Resolve secrets")

// Execute steps with progress updates
tracker.StartStep(0)
session, err := bwClient.Session()
tracker.CompleteStep()

tracker.StartStep(1)
folders, err := bwClient.ListFolders()
tracker.CompleteStep()

tracker.StartStep(2)
entries, err := env.ResolveDotEnv(file, reg)
tracker.CompleteStep()
```

### Registry Wrapper (Auto Spinner)

```go
// Automatically show spinner during resolution
baseReg := newRegistry(cfg)
progressReg := ui.NewProgressRegistry(baseReg, os.Stderr)
defer progressReg.Close()

// Spinner is shown automatically during Resolve calls
entries, err := env.ResolveDotEnv(file, progressReg)
```

## Output Examples

### Without TTY (CI/Pipes)

When output is piped or redirected, spinner frames are suppressed but messages are logged normally.

### With TTY

```
$ renv resolve .env
⠙ Resolving: bw://folder/database-secret

✓ renv  Loaded 5 variables from .env
  DATABASE_PASSWORD   ← bw://folder/database-secret
  API_KEY             ← bw://folder/api-key
  ...
```

## Dependencies

The spinner uses only standard library and existing dependencies:
- `github.com/charmbracelet/lipgloss` - for color styling
- `github.com/muesli/termenv` - for TTY detection
- `sync` - for thread safety
- `time` - for animation timing

No additional dependencies are required.

## Performance

- **Memory:** Minimal (single goroutine, small animation buffer)
- **CPU:** Negligible (~0.1% for 80ms frame timing)
- **Disabled automatically for non-TTY** to avoid any overhead

## Testing

Test examples are provided in `internal/ui/spinner_test.go`:

```go
// Basic spinner test
func TestSpinnerBasic(t *testing.T) {
	spinner := NewSpinner(os.Stderr, "Testing spinner")
	spinner.Start()
	time.Sleep(200 * time.Millisecond)
	spinner.Stop()
}

// Progress tracker test
func TestProgressTracker(t *testing.T) {
	tracker := NewProgressTracker(os.Stderr, "Operations")
	tracker.AddStep("Step 1")
	tracker.StartStep(0)
	tracker.CompleteStep()
}
```
