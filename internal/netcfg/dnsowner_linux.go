//go:build linux

package netcfg

import (
	"os"
	"strconv"
	"strings"
)

// ownerPIDKey marks which process pointed system DNS at its stub, so a second start can
// tell a crashed run's leftover from a live instance's settings.
const ownerPIDKey = "pid="

func ownerRecord() string { return ownerPIDKey + strconv.Itoa(os.Getpid()) }

// parseOwnerPID returns the pid from the first "pid=<n>" field in s, or 0.
func parseOwnerPID(s string) int {
	for _, f := range strings.Fields(s) {
		if v, ok := strings.CutPrefix(f, ownerPIDKey); ok {
			if pid, err := strconv.Atoi(v); err == nil && pid > 0 {
				return pid
			}
			return 0
		}
	}
	return 0
}
