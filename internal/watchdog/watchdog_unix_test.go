//go:build darwin || linux

package watchdog

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const parentModeEnv = "URNET_TEST_WATCHDOG_PARENT"

// TestMain turns this test binary into the two processes the integration tests need: a VPN
// stand-in that starts a real watchdog, and that watchdog (re-exec'd with Subcommand).
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == Subcommand {
		os.Exit(Main(os.Args[2:], func(owner int) error {
			fmt.Printf("recovered owner=%d\n", owner)
			return nil
		}))
	}
	switch os.Getenv(parentModeEnv) {
	case "kill", "release":
		runFakeParent(os.Getenv(parentModeEnv))
	}
	os.Exit(m.Run())
}

func runFakeParent(mode string) {
	h, err := Start(os.Getpid())
	if err != nil {
		fmt.Printf("start: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ready %d\n", h.pid)
	if mode == "release" {
		h.Release()
		os.Exit(0)
	}
	for {
		time.Sleep(time.Hour)
	}
}

// startFakeParent returns the parent, the watchdog's pid, and everything both write to stderr.
func startFakeParent(t *testing.T, mode string) (*exec.Cmd, int, <-chan string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), parentModeEnv+"="+mode)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = stderrW
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = stderrW.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	line, err := bufio.NewReader(stdout).ReadString('\n')
	pid, convErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "ready ")))
	if err != nil || convErr != nil {
		t.Fatalf("fake parent said %q, %v", line, err)
	}
	all := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(stderrR)
		all <- string(b)
	}()
	return cmd, pid, all
}

func waitOutput(t *testing.T, all <-chan string) string {
	t.Helper()
	select {
	case s := <-all:
		return s
	case <-time.After(10 * time.Second):
		t.Fatal("the watchdog did not exit")
		return ""
	}
}

func TestWatchdog_RecoversWhenParentIsKilled(t *testing.T) {
	parent, wdPID, all := startFakeParent(t, "kill")
	// Give the freshly exec'd watchdog time to reach signal.Ignore.
	time.Sleep(300 * time.Millisecond)
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM} {
		if err := syscall.Kill(wdPID, sig); err != nil {
			t.Fatalf("signal watchdog: %v", err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	out := waitOutput(t, all)
	if want := fmt.Sprintf("recovered owner=%d", parent.Process.Pid); !strings.Contains(out, want) {
		t.Fatalf("watchdog output = %q; want %q after SIGKILL of its parent, despite SIGINT, SIGHUP and SIGTERM", out, want)
	}
}

func TestWatchdog_ExitsQuietlyWhenReleased(t *testing.T) {
	parent, wdPID, all := startFakeParent(t, "release")
	if err := parent.Wait(); err != nil {
		t.Fatalf("fake parent: %v", err)
	}

	out := waitOutput(t, all)
	if strings.Contains(out, "recovered") {
		t.Fatalf("watchdog output = %q; a released watchdog must not recover", out)
	}
	if err := syscall.Kill(wdPID, 0); err == nil {
		time.Sleep(200 * time.Millisecond)
		if syscall.Kill(wdPID, 0) == nil {
			t.Fatal("the released watchdog is still running")
		}
	}
}
