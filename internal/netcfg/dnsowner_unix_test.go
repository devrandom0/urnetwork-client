//go:build darwin || linux

package netcfg

import (
	"os"
	"testing"
)

func usePIDAlive(t *testing.T, alive bool) {
	t.Helper()
	old := pidAlive
	pidAlive = func(int) bool { return alive }
	t.Cleanup(func() { pidAlive = old })
}

func TestPIDAlive(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Fatal("pidAlive(own pid) = false")
	}
	for _, pid := range []int{0, -1} {
		if pidAlive(pid) {
			t.Fatalf("pidAlive(%d) = true; kill(%d, 0) addresses a process group, not an owner", pid, pid)
		}
	}
}
