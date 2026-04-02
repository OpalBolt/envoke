package cleanup

// CleanupFunc is a function to call during cleanup events.
type CleanupFunc func() error

// Hook registers cleanup functions to be called on sleep, lock, and shutdown.
type Hook interface {
	Register(fns ...CleanupFunc) error
	Unregister()
}

// New returns the platform-appropriate Hook implementation.
func New() Hook {
	return newHook()
}
