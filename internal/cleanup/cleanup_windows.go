//go:build windows

package cleanup

import "log/slog"

// TODO: Implement using WTS session change events / Windows power events.

type windowsHook struct {
	fns []CleanupFunc
}

func newHook() Hook {
	return &windowsHook{}
}

func (h *windowsHook) Register(fns ...CleanupFunc) error {
	slog.Warn("cleanup: hooks not yet implemented on Windows; secrets will not be cleared on sleep/lock")
	h.fns = append(h.fns, fns...)
	return nil
}

func (h *windowsHook) Unregister() {}
