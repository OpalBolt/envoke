//go:build linux

package cleanup

import (
	"log"

	"github.com/godbus/dbus/v5"
)

type linuxHook struct {
	conn *dbus.Conn
	fns  []CleanupFunc
	done chan struct{}
}

func newHook() Hook {
	return &linuxHook{done: make(chan struct{})}
}

func (h *linuxHook) Register(fns ...CleanupFunc) error {
	h.fns = append(h.fns, fns...)
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		// Non-fatal: log and continue without hooks
		log.Printf("cleanup: cannot connect to D-Bus system bus: %v", err)
		return nil
	}
	h.conn = conn

	// Subscribe to PrepareForSleep signal (systemd-logind)
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.login1.Manager"),
		dbus.WithMatchMember("PrepareForSleep"),
	); err != nil {
		log.Printf("cleanup: cannot subscribe to PrepareForSleep: %v", err)
	}

	// Subscribe to Lock signal
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.login1.Session"),
		dbus.WithMatchMember("Lock"),
	); err != nil {
		log.Printf("cleanup: cannot subscribe to Lock: %v", err)
	}

	go h.listen()
	return nil
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
						h.runAll()
					}
				}
			} else if sig.Name == "org.freedesktop.login1.Session.Lock" {
				h.runAll()
			}
		case <-h.done:
			return
		}
	}
}

func (h *linuxHook) runAll() {
	for _, fn := range h.fns {
		if err := fn(); err != nil {
			log.Printf("cleanup: hook error: %v", err)
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
