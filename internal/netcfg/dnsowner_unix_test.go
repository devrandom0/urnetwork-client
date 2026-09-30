//go:build darwin || linux

package netcfg

import (
	"io"
	"os"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/logx"
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

// captureStderr returns what fn logs at WARN and ERROR.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	logx.SetLogLevel("info", false)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}
