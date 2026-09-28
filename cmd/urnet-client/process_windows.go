//go:build windows

package main

import "syscall"

// detachedProcAttr: windows has no session/Setsid concept; --background is unix-only at
// runtime, so this is a compile-time stub.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
