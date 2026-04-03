//go:build windows

package cleanup

import "log/slog"

// TODO: Implement using WTS session change events / Windows power events.

type windowsHook struct{}

func newHook() Hook {
	return &windowsHook{}
}

func (h *windowsHook) RegisterLock(fns ...CleanupFunc) error {
	slog.Warn("cleanup: hooks not yet implemented on Windows; secrets will not be cleared on lock")
	return nil
}

func (h *windowsHook) RegisterSleep(fns ...CleanupFunc) error {
	slog.Warn("cleanup: hooks not yet implemented on Windows; secrets will not be cleared on sleep")
	return nil
}

func (h *windowsHook) Unregister() {}
