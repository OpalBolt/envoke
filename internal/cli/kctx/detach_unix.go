//go:build !windows

package kctx

import (
	"log/slog"
	"syscall"
)

func detachFromTerminal() {
	if _, err := syscall.Setsid(); err != nil {
		slog.Debug("watch: setsid skipped", "reason", err)
	}
}
