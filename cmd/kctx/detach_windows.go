//go:build windows

package main

// detachFromTerminal is a no-op on Windows; session management is handled
// by the Windows console host and is not required for the watcher.
func detachFromTerminal() {}
