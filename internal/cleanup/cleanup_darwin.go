//go:build darwin

package cleanup

import "log"

// TODO: Implement using IOKit/NSWorkspace for sleep and lock events.

type darwinHook struct {
	fns []CleanupFunc
}

func newHook() Hook {
	return &darwinHook{}
}

func (h *darwinHook) Register(fns ...CleanupFunc) error {
	log.Println("cleanup: hooks not yet implemented on Darwin; secrets will not be cleared on sleep/lock")
	h.fns = append(h.fns, fns...)
	return nil
}

func (h *darwinHook) Unregister() {}
