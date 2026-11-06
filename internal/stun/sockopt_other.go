//go:build !linux && !windows

package stun

import (
	"syscall"
)

func reuseControl(_, _ string, c syscall.RawConn) error {
	var setErr error
	err := c.Control(func(fd uintptr) {
		if e := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); e != nil {
			setErr = e
			return
		}
		if e := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1); e != nil {
			setErr = e
		}
	})
	if err != nil {
		return err
	}
	return setErr
}
