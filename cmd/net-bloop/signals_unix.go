//go:build !windows

package main

import (
	"os"
	"syscall"
)

func reloadSignal() os.Signal { return syscall.SIGHUP }

func isReloadSignal(s os.Signal) bool { return s == syscall.SIGHUP }
