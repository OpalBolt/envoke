//go:build darwin

package cleanup

import (
	"log/slog"
	"sync"
	"time"
)

// darwinHook detects sleep/wake transitions using timer drift.
// When the system sleeps the goroutine is suspended; on wake the next tick
// fires with a gap larger than 2× the poll interval, indicating a sleep/wake
// cycle occurred.
//
// Screen lock events are not detectable in pure Go without CGo/IOKit.
type darwinHook struct {
	mu       sync.Mutex
	sleepFns []CleanupFunc
	done     chan struct{}
	started  bool
}

const pollInterval = 10 * time.Second

func newHook() Hook {
	return &darwinHook{done: make(chan struct{})}
}

// RegisterLock is a no-op on macOS: screen lock detection requires CGo/IOKit.
func (h *darwinHook) RegisterLock(fns ...CleanupFunc) error {
	slog.Debug("cleanup: screen lock detection not available on macOS without CGo; lock hooks ignored")
	return nil
}

func (h *darwinHook) RegisterSleep(fns ...CleanupFunc) error {
	h.mu.Lock()
	h.sleepFns = append(h.sleepFns, fns...)
	alreadyStarted := h.started
	if !alreadyStarted {
		h.started = true
	}
	h.mu.Unlock()

	if !alreadyStarted {
		slog.Debug("cleanup: darwin sleep watcher started (timer drift)")
		go h.poll()
	}
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
				h.runSleep()
			}
			last = t
		}
	}
}

func (h *darwinHook) runSleep() {
	h.mu.Lock()
	fns := append([]CleanupFunc(nil), h.sleepFns...)
	h.mu.Unlock()
	for _, fn := range fns {
		if err := fn(); err != nil {
			slog.Warn("cleanup: sleep hook error", "error", err)
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
