//go:build linux

package securedir

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir returns the most-secure writable directory for session-scoped files on Linux.
//
// Priority:
//  1. /run/user/<uid> — systemd-logind tmpfs, mode 0700; other local users
//     cannot list filenames, unlike /dev/shm.
//  2. /dev/shm — world-listable but RAM-backed and cleared on reboot.
//  3. /tmp — universal fallback.
func Dir() string {
	uid := os.Getuid()

	// /run/user/<uid> is created by systemd-logind as 0700.
	runUser := fmt.Sprintf("/run/user/%d", uid)
	if fi, err := os.Stat(runUser); err == nil && fi.IsDir() {
		probe := filepath.Join(runUser, ".envoke-probe")
		if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(probe)
			return runUser
		}
	}

	// /dev/shm is world-listable but still RAM-backed.
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		probe := filepath.Join("/dev/shm", ".envoke-probe")
		if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			f.Close()
			os.Remove(probe)
			return "/dev/shm"
		}
	}

	return "/tmp"
}
