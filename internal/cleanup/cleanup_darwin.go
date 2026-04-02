//go:build darwin

package cleanup

import (
	"log/slog"
	"time"
)

// darwinHook detects sleep/wake transitions using timer drift.
// When the system sleeps the goroutine is suspended; on wake the next tick
// fires with a gap larger than 2× the poll interval, indicating a sleep/wake
// cycle occurred.
//
// Screen lock events are not detectable in pure Go without CGo/IOKit.
type darwinHook struct {
	fns  []CleanupFunc
	done chan struct{}
}

const pollInterval = 10 * time.Second

func newHook() Hook {
	return &darwinHook{done: make(chan struct{})}
}

func (h *darwinHook) Register(fns ...CleanupFunc) error {
	h.fns = append(h.fns, fns...)
	slog.Debug("cleanup: darwin sleep watcher started (timer drift)")
	go h.poll()
	return nil
}

func (h *darwinHook) poll() {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case <-h.done:
			return
		case t := <-ticker.C:
			// If more than 2× the interval has elapsed the goroutine was suspended —
			// the system slept and just woke up.
			if t.Sub(last) > 2*pollInterval {
				slog.Debug("cleanup: sleep/wake detected via timer drift, running hooks")
				h.runAll()
			}
			last = t
		}
	}
}

func (h *darwinHook) runAll() {
	for _, fn := range h.fns {
		if err := fn(); err != nil {
			slog.Warn("cleanup: hook error", "error", err)
		}
	}
}

func (h *darwinHook) Unregister() {
	if h.done != nil {
		select {
		case <-h.done:
		default:
			close(h.done)
		}
	}
}

var _ Hook = (*darwinHook)(nil)
