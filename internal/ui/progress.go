// Package ui progress helpers for showing spinner and status during long operations.
package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// ProgressWriter shows a spinner with status message during long operations.
type ProgressWriter struct {
	mu       sync.Mutex
	w        io.Writer
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	status   string
	frames   []string
	interval time.Duration
	active   bool
}

// NewProgressWriter creates a progress writer that shows a spinner on TTY.
func NewProgressWriter(w io.Writer) *ProgressWriter {
	if f, ok := w.(*os.File); !ok || !isTTY(f) {
		return &ProgressWriter{
			w:    w,
			done: make(chan struct{}),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	pw := &ProgressWriter{
		w:      w,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		frames: []string{
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		},
		interval: 80 * time.Millisecond,
	}
	return pw
}

// isTTY checks if a file is a TTY.
func isTTY(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// Start begins showing the spinner with the given status message.
func (pw *ProgressWriter) Start(status string) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if pw.active {
		return
	}

	pw.status = status
	pw.active = true

	if f, ok := pw.w.(*os.File); ok && isTTY(f) {
		go pw.spin()
	}
}

// Update changes the status message without stopping the spinner.
func (pw *ProgressWriter) Update(status string) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.status = status
}

// Done stops the spinner and clears the line.
func (pw *ProgressWriter) Done() {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if !pw.active {
		return
	}

	pw.active = false
	pw.cancel()
	<-pw.done

	if f, ok := pw.w.(*os.File); ok && isTTY(f) {
		fmt.Fprint(pw.w, "\r\033[K")
	}
}

// spin runs in a goroutine and continuously renders the spinner.
func (pw *ProgressWriter) spin() {
	defer close(pw.done)

	ticker := time.NewTicker(pw.interval)
	defer ticker.Stop()

	frameIdx := 0
	for {
		select {
		case <-pw.ctx.Done():
			return
		case <-ticker.C:
			pw.mu.Lock()
			if pw.active {
				frame := pw.frames[frameIdx%len(pw.frames)]
				line := fmt.Sprintf("\r%s %s", frame, pw.status)
				fmt.Fprint(pw.w, line)
				frameIdx++
			}
			pw.mu.Unlock()
		}
	}
}

// StatusLine shows a simple status message on stderr.
type StatusLine struct {
	w      io.Writer
	mu     sync.Mutex
	active bool
}

// NewStatusLine creates a status line printer.
func NewStatusLine(w io.Writer) *StatusLine {
	return &StatusLine{w: w}
}

// Set displays a status message.
func (sl *StatusLine) Set(status string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if f, ok := sl.w.(*os.File); ok && isTTY(f) {
		fmt.Fprintf(sl.w, "\r\033[K%s", status)
		sl.active = true
	}
}

// Clear removes the status message.
func (sl *StatusLine) Clear() {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.active {
		if f, ok := sl.w.(*os.File); ok && isTTY(f) {
			fmt.Fprint(sl.w, "\r\033[K")
			sl.active = false
		}
	}
}
