//go:build darwin || linux

package watchdog

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const parentModeEnv = "URNET_TEST_WATCHDOG_PARENT"

// recoveredMarker is relative: the watchdog runs with an empty environment, so the only
// place a test can find what it did is the working directory it inherits.
func recoveredMarker(owner int) string { return fmt.Sprintf("recovered-%d", owner) }

// TestMain turns this test binary into the two processes the integration tests need: a VPN
// stand-in that starts a real watchdog, and that watchdog (re-exec'd with Subcommand).
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == Subcommand {
		os.Exit(Main(os.Args[2:], func(owner int) error {
			return os.WriteFile(recoveredMarker(owner), nil, 0o600)
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

// startFakeParent starts a VPN stand-in in dir with stderr on a pipe, and returns it, the
// watchdog's pid and the read end of that stderr pipe.
func startFakeParent(t *testing.T, mode, dir string) (*exec.Cmd, int, *os.File) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), parentModeEnv+"="+mode)
	cmd.Dir = dir
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
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = stderrR.Close() })

	line, err := bufio.NewReader(stdout).ReadString('\n')
	pid, convErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "ready ")))
	if err != nil || convErr != nil {
		t.Fatalf("fake parent said %q, %v", line, err)
	}
	return cmd, pid, stderrR
}

func waitRecovered(t *testing.T, dir string, owner int) {
	t.Helper()
	path := filepath.Join(dir, recoveredMarker(owner))
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the watchdog did not recover for pid %d", owner)
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("watchdog pid %d is still running", pid)
}

func TestWatchdog_RecoversWhenParentIsKilled(t *testing.T) {
	dir := t.TempDir()
	parent, wdPID, _ := startFakeParent(t, "kill", dir)
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

	waitRecovered(t, dir, parent.Process.Pid)
}

// The original incident's shape: the process that must restore DNS dies on its first log
// line because whatever was reading its output (a `| tee`, a closed terminal) is gone.
func TestWatchdog_RecoversWhenItsStderrReaderIsGone(t *testing.T) {
	dir := t.TempDir()
	parent, _, stderrR := startFakeParent(t, "kill", dir)
	time.Sleep(300 * time.Millisecond)
	_ = stderrR.Close()

	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	waitRecovered(t, dir, parent.Process.Pid)
}

// Covers the SIGPIPE handling on its own: a watchdog whose stderr is a broken pipe, the case
// Start's choice of output avoids, must still log its warning, recover and exit cleanly.
func TestWatchdogMain_SurvivesBrokenStderrPipe(t *testing.T) {
	dir := t.TempDir()
	ctlR, ctlW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = outR.Close()
	const owner = 424242
	cmd := exec.Command(os.Args[0], Subcommand, "--owner", strconv.Itoa(owner))
	cmd.Dir = dir
	cmd.ExtraFiles = []*os.File{ctlR}
	cmd.Stdout, cmd.Stderr = outW, outW
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = ctlR.Close()
	_ = outW.Close()
	time.Sleep(300 * time.Millisecond)
	_ = ctlW.Close()

	err = cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		t.Fatalf("watchdog ended with %v (signal %v); a dead log reader must not stop recovery", err, exit.Sys().(syscall.WaitStatus).Signal())
	}
	if err != nil {
		t.Fatal(err)
	}
	waitRecovered(t, dir, owner)
}

func TestWatchdog_ExitsQuietlyWhenReleased(t *testing.T) {
	dir := t.TempDir()
	parent, wdPID, _ := startFakeParent(t, "release", dir)
	if err := parent.Wait(); err != nil {
		t.Fatalf("fake parent: %v", err)
	}

	waitGone(t, wdPID)
	if _, err := os.Stat(filepath.Join(dir, recoveredMarker(parent.Process.Pid))); err == nil {
		t.Fatal("a released watchdog recovered DNS")
	}
}

func TestOutputFor_KeepsALogFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "urnet.log")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if out := outputFor(f); out != f {
		t.Fatal("want the --log_file the VPN logs to")
	}
}

func TestOutputFor_KeepsATerminal(t *testing.T) {
	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tty.Close() }()

	if out := outputFor(tty); out != tty {
		t.Fatal("a character device such as a terminal raises no SIGPIPE; keep it")
	}
}

func TestOutputFor_ReplacesAPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()

	if out := outputFor(w); out != nil {
		t.Fatalf("output = %s; want none (the null device): a pipe's reader can go away", out.Name())
	}
}
