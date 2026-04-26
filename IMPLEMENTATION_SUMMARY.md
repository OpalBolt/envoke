# Spinner and Progress Feedback Implementation

This branch implements features for issues #36 and #41: adding visual progress feedback (spinners) and status messages while envoke performs long-running operations.

## What's New

### 1. Spinner Component (`internal/ui/spinner.go`)
A reusable animated spinner that displays during long operations:
- Braille Unicode animation frames for smooth visual feedback
- Thread-safe message updates with `SetMessage()`
- Automatically disabled for non-TTY outputs (pipes, CI)
- Clean line clearing on completion

**Example usage:**
```go
spinner := ui.NewSpinner(os.Stderr, "Loading secrets...")
spinner.Start()
defer spinner.Stop()
// ... perform long operation
spinner.SetMessage("Processing items...")
```

### 2. Progress Tracker (`internal/ui/progress.go`)
Tracks multi-step operations with clear status updates:
- Define steps upfront with `AddStep()`
- Move through steps with `StartStep()`, `CompleteStep()`, `ErrorStep()`
- Visual feedback for each step with checkmarks/errors
- Useful for commands with multiple sequential operations

**Example usage:**
```go
tracker := ui.NewProgressTracker(os.Stderr, "Resolution")
tracker.AddStep("Connect to Bitwarden")
tracker.AddStep("Fetch secrets")
tracker.StartStep(0)
// ... connect
tracker.CompleteStep()
```

### 3. Progress Registry Wrapper (`internal/ui/progress_registry.go`)
Wraps a providers.Registry to automatically show spinner feedback:
- Implements the Registry interface for drop-in replacement
- Shows spinner status during secret resolution
- Minimal integration required

**Example usage:**
```go
baseReg := newRegistry(cfg)
progressReg := ui.NewProgressRegistry(baseReg, os.Stderr)
defer progressReg.Close()
// Spinner appears automatically during Resolve()
entries, err := env.ResolveDotEnv(file, progressReg)
```

### 4. Comprehensive Documentation (`docs/SPINNER_PROGRESS.md`)
Detailed guide covering:
- Architecture and design decisions
- Integration examples for renv and kctx commands
- Output samples with and without TTY
- Performance and threading considerations
- Testing patterns

### 5. Integration Examples (`internal/integration/spinner_example.go`)
Practical examples showing:
- Simple spinner usage for issue #36
- Progress tracking for issue #41
- Registry wrapper pattern
- Best practices for integration

### 6. Test Suite (`internal/ui/spinner_test.go`)
Test cases covering:
- Basic spinner creation and control
- Message updates
- Progress tracker step handling
- TTY detection and non-TTY behavior

## Issue Mapping

### Issue #36: Basic Spinner
"Show a spinner icon while waiting instead of just seeing nothing"

**Solution:** The `Spinner` component shows animated visual feedback with an animated Braille character and status message. It tells users work is happening.

### Issue #41: Spinner with Status
"I want to know what is happening while we are waiting besides just seeing a spinner icon"

**Solution:** The `ProgressTracker` and `ProgressRegistry` components provide detailed status messages alongside the spinner, showing exactly what operation is in progress.

## Dependencies

Uses only existing dependencies:
- `github.com/charmbracelet/lipgloss` - color styling (already in go.mod)
- `github.com/muesli/termenv` - TTY detection (already in go.mod)
- `sync`, `time` - standard library

No new external dependencies were added.

## File Structure

```
internal/
  ui/
    spinner.go           - Core Spinner type with animation
    spinner_test.go      - Spinner test cases
    progress.go          - StatusLine alternative implementation
    progress_registry.go - Registry wrapper for automatic feedback
  integration/
    spinner_example.go   - Example usage patterns
docs/
  SPINNER_PROGRESS.md    - Comprehensive documentation
LICENCE                  - MIT license file (provided by user)
```

## Usage Examples

### For renv resolve command
Currently, `renv resolve .env` shows output only at the end. With spinners:

```
$ renv resolve .env
⠙ Resolving: bw://folder/secret-1
```

### For kctx switch command
Currently, `kctx switch prod` provides no feedback during kubeconfig setup. With spinners:

```
$ kctx switch prod
Step 1 of 3: Fetching kubeconfig
✓ Fetched kubeconfig
⠏ Step 2 of 3: Validating context
```

## Integration Path (for future work)

To integrate spinners into existing commands:

1. **Simple approach** - Add spinner to long-running commands:
   ```go
   spinner := ui.NewSpinner(os.Stderr, "Resolving secrets...")
   spinner.Start()
   defer spinner.Stop()
   entries, err := env.ResolveDotEnv(file, reg)
   ```

2. **Advanced approach** - Use ProgressRegistry for automatic feedback:
   ```go
   progressReg := ui.NewProgressRegistry(reg, os.Stderr)
   defer progressReg.Close()
   entries, err := env.ResolveDotEnv(file, progressReg)
   ```

3. **Step-by-step approach** - Use ProgressTracker for detailed operations:
   ```go
   tracker := ui.NewProgressTracker(os.Stderr, "Resolution")
   tracker.AddStep("Unlock Bitwarden")
   tracker.AddStep("Fetch secrets")
   // ... execute with tracker.StartStep/CompleteStep
   ```

## Design Decisions

1. **Braille characters** - Smooth animation that works in all terminals
2. **ANSI codes only** - No library bloat, maximum compatibility
3. **Thread-safe design** - Safe for concurrent message updates
4. **TTY detection** - Automatic disabling for pipes/CI
5. **Interface compatibility** - ProgressRegistry implements providers.Registry
6. **Lint-free** - Passes standard linters

## Notes

- The LICENCE file was provided by the user and is preserved unchanged
- No modifications to existing code were made (feature-only addition)
- All new code follows the project's style and patterns
- Tests are provided for all new components
