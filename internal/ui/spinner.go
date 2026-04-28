// Package ui provides styled output helpers for CLI feedback using lipgloss.
// This file implements a spinner for long-running operations.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Spinner provides animated progress feedback during long-running operations.
// Only renders to TTY outputs — silently suppressed for pipes, CI, and direnv.
type Spinner struct {
	writer     io.Writer
	message    string
	isTerminal bool
	frames     []string
	frameIdx   int
	ticker     *time.Ticker
	done       chan struct{}
	stopped    bool
	mu         sync.Mutex
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates a spinner writing to w with the given initial message.
func NewSpinner(w io.Writer, message string) *Spinner {
	isTerminal := false
	if f, ok := w.(*os.File); ok {
		isTerminal = term.IsTerminal(int(f.Fd()))
	}
	return &Spinner{
		writer:     w,
		message:    message,
		isTerminal: isTerminal,
		frames:     spinnerFrames,
		done:       make(chan struct{}),
	}
}

// Start begins the spinner animation. No-op if already running or not a TTY.
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ticker != nil || !s.isTerminal {
		return
	}
	s.ticker = time.NewTicker(80 * time.Millisecond)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.mu.Lock()
				s.render()
				s.mu.Unlock()
			case <-s.done:
				return
			}
		}
	}()
}

// Stop ends the animation and clears the spinner line. Safe to call multiple times.
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	if !s.stopped {
		close(s.done)
		s.stopped = true
	}
	if s.isTerminal {
		fmt.Fprintf(s.writer, "\r\033[K")
	}
}

// SetMessage updates the displayed message. Thread-safe.
func (s *Spinner) SetMessage(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = msg
	s.render()
}

// render prints the current frame + message. Must be called with s.mu held.
func (s *Spinner) render() {
	if !s.isTerminal {
		return
	}
	frame := s.frames[s.frameIdx%len(s.frames)]
	s.frameIdx++
	r := rendererFor(s.writer)
	spinStyle := r.NewStyle().Foreground(lipgloss.Color("6"))
	// \033[K clears from cursor to end of line, avoiding leftover chars from longer previous messages
	fmt.Fprintf(s.writer, "\r%s %s\033[K", spinStyle.Render(frame), s.message)
}
