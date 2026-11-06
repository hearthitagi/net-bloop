//go:build linux

package stun

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// reuseControl allows a listener and an outbound socket to share the same local
// port, which is required for TCP STUN hole punching on Linux.
func reuseControl(_, _ string, c syscall.RawConn) error {
	var setErr error
	err := c.Control(func(fd uintptr) {
		if e := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); e != nil {
			setErr = e
			return
		}
		if e := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); e != nil {
			setErr = e
		}
	})
	if err != nil {
		return err
	}
	return setErr
}
