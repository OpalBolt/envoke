//go:build !windows

package main

import (
	"log/slog"
	"syscall"
)

// detachFromTerminal moves the process into a new session so it is no longer
// a member of the calling shell's process group.  This prevents Ctrl+C
// (SIGINT sent to the foreground process group) from killing the watcher.
func detachFromTerminal() {
	if _, err := syscall.Setsid(); err != nil {
		// Setsid fails when the process is already a process-group leader,
		// which can happen if the shell put the background job in its own
		// group (job-control enabled).  That's fine — the goal is achieved.
		slog.Debug("watch: setsid skipped", "reason", err)
	}
}
