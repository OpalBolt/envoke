//go:build linux

package cleanup

import (
	"os"
	"sync"

	"log/slog"

	"github.com/godbus/dbus/v5"
)

type linuxHook struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	lockFns  []CleanupFunc
	sleepFns []CleanupFunc
	done     chan struct{}
	started  bool
}

func newHook() Hook {
	return &linuxHook{done: make(chan struct{})}
}

func (h *linuxHook) RegisterLock(fns ...CleanupFunc) error {
	h.mu.Lock()
	h.lockFns = append(h.lockFns, fns...)
	h.mu.Unlock()
	return h.ensureStarted()
}

func (h *linuxHook) RegisterSleep(fns ...CleanupFunc) error {
	h.mu.Lock()
	h.sleepFns = append(h.sleepFns, fns...)
	h.mu.Unlock()
	return h.ensureStarted()
}

// ensureStarted connects to D-Bus and starts the listener goroutine once.
func (h *linuxHook) ensureStarted() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return nil
	}
	h.started = true

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		// Non-fatal: log and continue without hooks
		slog.Warn("cleanup: cannot connect to D-Bus system bus", "error", err)
		return nil
	}
	h.conn = conn

	// Subscribe to PrepareForSleep signal (systemd-logind)
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.login1.Manager"),
		dbus.WithMatchMember("PrepareForSleep"),
	); err != nil {
		slog.Warn("cleanup: cannot subscribe to PrepareForSleep", "error", err)
	}

	// Resolve the current session object path so we subscribe to Lock signals
	// for exactly this session. Without the path, logind does not route the
	// per-session signal to us.
	sessionPath := h.currentSessionPath(conn)
	lockOpts := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.login1.Session"),
		dbus.WithMatchMember("Lock"),
	}
	if sessionPath != "" {
		lockOpts = append(lockOpts, dbus.WithMatchObjectPath(sessionPath))
	}
	if err := conn.AddMatchSignal(lockOpts...); err != nil {
		slog.Warn("cleanup: cannot subscribe to Lock", "error", err)
	}

	go h.listen()
	return nil
}

// currentSessionPath returns the D-Bus object path of the current login session.
func (h *linuxHook) currentSessionPath(conn *dbus.Conn) dbus.ObjectPath {
	obj := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1")
	var path dbus.ObjectPath
	if err := obj.Call("org.freedesktop.login1.Manager.GetSessionByPID", 0, uint32(os.Getpid())).Store(&path); err != nil {
		slog.Debug("cleanup: cannot resolve session path, Lock match will be broad", "error", err)
		return ""
	}
	slog.Debug("cleanup: resolved session path for Lock subscription", "path", path)
	return path
}

func (h *linuxHook) listen() {
	c := make(chan *dbus.Signal, 10)
	h.conn.Signal(c)
	for {
		select {
		case sig := <-c:
			if sig == nil {
				return
			}
			// PrepareForSleep is emitted with true before sleep, false after wake
			if sig.Name == "org.freedesktop.login1.Manager.PrepareForSleep" {
				if len(sig.Body) > 0 {
					if going, ok := sig.Body[0].(bool); ok && going {
						h.runSleep()
					}
				}
			} else if sig.Name == "org.freedesktop.login1.Session.Lock" {
				h.runLock()
			}
		case <-h.done:
			return
		}
	}
}

func (h *linuxHook) runLock() {
	h.mu.Lock()
	fns := append([]CleanupFunc(nil), h.lockFns...)
	h.mu.Unlock()
	for _, fn := range fns {
		if err := fn(); err != nil {
			slog.Warn("cleanup: lock hook error", "error", err)
		}
	}
}

func (h *linuxHook) runSleep() {
	h.mu.Lock()
	fns := append([]CleanupFunc(nil), h.sleepFns...)
	h.mu.Unlock()
	for _, fn := range fns {
		if err := fn(); err != nil {
			slog.Warn("cleanup: sleep hook error", "error", err)
		}
	}
}

func (h *linuxHook) Unregister() {
	if h.done != nil {
		select {
		case <-h.done:
		default:
			close(h.done)
		}
	}
	if h.conn != nil {
		h.conn.Close()
	}
}

// Ensure linuxHook implements Hook.
var _ Hook = (*linuxHook)(nil)
