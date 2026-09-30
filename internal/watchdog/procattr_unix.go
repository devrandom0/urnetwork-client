//go:build unix

package watchdog

import "syscall"

// detachedProcAttr gives the watchdog its own session, so the terminal's signals and its
// closing never reach it.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
