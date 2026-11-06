//go:build windows

package main

import "os"

func reloadSignal() os.Signal { return nil }

func isReloadSignal(os.Signal) bool { return false }
