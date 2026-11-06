//go:build windows

package stun

import (
	"syscall"
)

// reuseControl enables SO_REUSEADDR so an outbound socket can try to share the
// listener port. Windows semantics are looser than Linux; if same-port hopping
// fails the runtime reports it and falls back to NAT-PMP/UPnP.
func reuseControl(_, _ string, c syscall.RawConn) error {
	var setErr error
	err := c.Control(func(fd uintptr) {
		setErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return setErr
}
