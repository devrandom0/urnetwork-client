//go:build darwin || linux

package netcfg

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// pidAlive treats EPERM as alive: the process exists but belongs to another user.
var pidAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// liveOwnerErr reports a live owner of system DNS other than this process.
func liveOwnerErr(pid int) error {
	if pid <= 0 || pid == os.Getpid() || !pidAlive(pid) {
		return nil
	}
	return fmt.Errorf("another urnet-client (pid %d) owns system DNS; stop it first", pid)
}
