//go:build unix

package main

import "syscall"

// detachedProcAttr puts the background child in its own session so it survives the
// parent's terminal closing.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// backgroundSupported: --background detaches via Setsid, which unix always has.
func backgroundSupported() error {
	return nil
}
