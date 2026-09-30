//go:build windows

package watchdog

import "syscall"

// detachedProcAttr: the VPN does not run on windows, so nothing starts a watchdog there.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
