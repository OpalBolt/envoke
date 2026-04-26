// Package ui provides styled output helpers for CLI feedback using lipgloss.
// This file implements spinners and progress indicators for long-running operations.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Spinner provides animated progress feedback during long-running operations.
// It renders status messages with animated spinner frames.
type Spinner struct {
	writer     io.Writer
	message    string
	isTerminal bool
	frames     []string
	frameIdx   int
	ticker     *time.Ticker
	done       chan struct{}
	mu         sync.Mutex
}

// spinnerFrames are the animation frames for the spinner.
// Uses a simple rotating dots pattern that works well in all terminals.
var spinnerFrames = []string{
	"⠋",
	"⠙",
	"⠹",
	"⠸",
	"⠼",
	"⠴",
	"⠦",
	"⠧",
	"⠇",
	"⠏",
}

// NewSpinner creates a new spinner that outputs to w with the given message.
func NewSpinner(w io.Writer, message string) *Spinner {
	// Check if writer is a terminal for TTY detection
	isTerminal := false
	if f, ok := w.(*os.File); ok {
		isTerminal = termenv.File(f).Color().Has256 || termenv.File(f).Color().Has16m
	}

	return &Spinner{
		writer:     w,
		message:    message,
		isTerminal: isTerminal,
		frames:     spinnerFrames,
		frameIdx:   0,
		done:       make(chan struct{}),
	}
}

// Start begins the spinner animation. Call Stop() to end it.
// Safe to call multiple times (subsequent calls are no-ops if already running).
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ticker != nil {
		return // already running
	}

	s.ticker = time.NewTicker(80 * time.Millisecond)

	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.render()
			case <-s.done:
				return
			}
		}
	}()
}

// Stop ends the spinner animation and clears the line.
// Safe to call even if spinner was never started.
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	close(s.done)

	// Clear the spinner line
	if s.isTerminal {
		fmt.Fprintf(s.writer, "\r\033[K")
	}
}

// SetMessage updates the message displayed with the spinner.
// Thread-safe.
func (s *Spinner) SetMessage(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = msg
	s.render()
}

// render prints the current frame and message to the writer.
// Must be called with the lock held.
func (s *Spinner) render() {
	if !s.isTerminal {
		return // don't render to non-TTY
	}

	frame := s.frames[s.frameIdx%len(s.frames)]
	s.frameIdx++

	r := rendererFor(s.writer)
	spinStyle := r.NewStyle().Foreground(lipgloss.Color("6")) // cyan

	// Format: "⠋ Fetching secrets from Bitwarden..."
	output := fmt.Sprintf("\r%s %s", spinStyle.Render(frame), s.message)

	// Pad to clear previous longer lines
	output = fmt.Sprintf("%%-80s", output)

	fmt.Fprint(s.writer, output)
}

// ProgressTracker tracks progress through steps of a longer operation.
// Useful for providing detailed feedback about what is happening.
type ProgressTracker struct {
	writer      io.Writer
	title       string
	steps       []string
	currentStep int
	isTerminal  bool
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(w io.Writer, title string) *ProgressTracker {
	isTerminal := false
	if f, ok := w.(*os.File); ok {
		isTerminal = termenv.File(f).Color().Has256 || termenv.File(f).Color().Has16m
	}

	return &ProgressTracker{
		writer:      w,
		title:       title,
		steps:       make([]string, 0),
		currentStep: -1,
		isTerminal:  isTerminal,
	}
}

// AddStep adds a step to the progress tracker.
func (pt *ProgressTracker) AddStep(step string) {
	pt.steps = append(pt.steps, step)
}

// StartStep begins a specific step. Should be followed by CompleteStep or ErrorStep.
func (pt *ProgressTracker) StartStep(stepNum int) {
	if stepNum < 0 || stepNum >= len(pt.steps) {
		return
	}
	pt.currentStep = stepNum
	pt.render()
}

// CompleteStep marks the current step as complete and moves to the next.
func (pt *ProgressTracker) CompleteStep() {
	if pt.currentStep >= 0 && pt.currentStep < len(pt.steps) {
		step := pt.steps[pt.currentStep]
		Success(pt.writer, step)
	}
	pt.currentStep = -1
}

// ErrorStep marks the current step with an error.
func (pt *ProgressTracker) ErrorStep(err error) {
	if pt.currentStep >= 0 && pt.currentStep < len(pt.steps) {
		step := pt.steps[pt.currentStep]
		Error(pt.writer, fmt.Sprintf("%s: %v", step, err))
	}
	pt.currentStep = -1
}

// render prints the current progress state.
func (pt *ProgressTracker) render() {
	if !pt.isTerminal || pt.currentStep < 0 {
		return
	}

	r := rendererFor(pt.writer)
	progressStyle := r.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	_ = progressStyle

	// Show progress: "Step 1 of 3: Fetching..."
	current := pt.currentStep + 1
	total := len(pt.steps)
	step := pt.steps[pt.currentStep]

	progressInfo := fmt.Sprintf("Step %d of %d: %s", current, total, step)
	fmt.Fprintf(pt.writer, "\r%s\n", progressInfo)
}