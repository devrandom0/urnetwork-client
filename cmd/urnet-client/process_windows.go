//go:build windows

package main

import (
	"errors"
	"syscall"
)

// detachedProcAttr: windows has no session/Setsid concept; backgroundSupported refuses
// --background before spawnBackground ever reaches this, so it is a compile-time stub only.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// backgroundSupported refuses --background outright: without detachedProcAttr's Setsid
// equivalent, spawnBackground would start a plain, non-detached child instead of one that
// survives the parent's terminal closing.
func backgroundSupported() error {
	return errors.New("--background is not supported on windows")
}
