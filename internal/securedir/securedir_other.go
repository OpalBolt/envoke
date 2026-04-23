//go:build !linux && !darwin && !windows

package securedir

// Dir returns /tmp as a safe fallback on unsupported platforms.
func Dir() string {
	return "/tmp"
}
