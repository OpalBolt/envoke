package cleanup

// CleanupFunc is a function to call during cleanup events.
type CleanupFunc func() error

// Hook registers cleanup functions to be called on specific system events.
// Use RegisterLock for functions that should run when the screen is locked,
// and RegisterSleep for functions that should run when the system suspends.
type Hook interface {
	// RegisterLock registers functions to call when the screen is locked.
	RegisterLock(fns ...CleanupFunc) error
	// RegisterSleep registers functions to call when the system suspends/sleeps.
	RegisterSleep(fns ...CleanupFunc) error
	Unregister()
}

// New returns the platform-appropriate Hook implementation.
func New() Hook {
	return newHook()
}
