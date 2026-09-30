package netcfg

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeResult struct {
	out    string // combined stdout+stderr, scripted for Capture
	stdout string // stdout only, scripted for CaptureStdout
	err    error
}

// fakeRunner records every command and returns scripted results; safe for concurrent use.
type fakeRunner struct {
	mu      sync.Mutex
	calls   []string
	inputs  []string
	results map[string]fakeResult
	// byInput scripts RunInput by its stdin, for tools like scutil that take commands there.
	byInput map[string]fakeResult
}

func useFakeRunner(t *testing.T) *fakeRunner {
	t.Helper()
	f := &fakeRunner{results: map[string]fakeResult{}, byInput: map[string]fakeResult{}}
	old := cmdRunner
	cmdRunner = f
	t.Cleanup(func() { cmdRunner = old })
	return f
}

func (f *fakeRunner) failOn(line, out string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[line] = fakeResult{out: out, err: errors.New("exit status 2")}
}

func (f *fakeRunner) Run(name string, args ...string) error {
	_, err := f.Capture(name, args...)
	return err
}

func (f *fakeRunner) Capture(name string, args ...string) (string, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, line)
	r := f.results[line]
	return r.out, r.err
}

func (f *fakeRunner) CaptureStdout(name string, args ...string) (string, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, line)
	r := f.results[line]
	return r.stdout, r.err
}

func (f *fakeRunner) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeRunner) count(line string) int {
	n := 0
	for _, c := range f.Calls() {
		if c == line {
			n++
		}
	}
	return n
}

func (f *fakeRunner) RunInput(stdin, name string, args ...string) (string, error) {
	f.mu.Lock()
	f.inputs = append(f.inputs, stdin)
	r, ok := f.byInput[stdin]
	f.mu.Unlock()
	if ok {
		line := strings.Join(append([]string{name}, args...), " ")
		f.mu.Lock()
		f.calls = append(f.calls, line)
		f.mu.Unlock()
		return r.out, r.err
	}
	return f.Capture(name, args...)
}

// Inputs returns the stdin of every RunInput call, in order.
func (f *fakeRunner) Inputs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.inputs...)
}

func TestExecRunner_RunInputFeedsStdin(t *testing.T) {
	out, err := execRunner{}.RunInput("hello from stdin\n", "cat")
	if err != nil || out != "hello from stdin\n" {
		t.Fatalf("RunInput = %q, %v; want stdin echoed by cat", out, err)
	}
}

func TestRunCaptureUsesCommandRunner(t *testing.T) {
	f := useFakeRunner(t)
	f.results["echo hi"] = fakeResult{out: "scripted"}
	out, err := runCapture("echo", "hi")
	if err != nil || out != "scripted" {
		t.Fatalf("runCapture = %q, %v; want scripted output from the fake", out, err)
	}
	if got := f.Calls(); len(got) != 1 || got[0] != "echo hi" {
		t.Fatalf("calls = %v", got)
	}
}

func TestRunCaptureStdoutIgnoresStderr(t *testing.T) {
	f := useFakeRunner(t)
	f.results["echo hi"] = fakeResult{stdout: "stdout-only", out: "stdout-only\nstderr junk"}
	out, err := runCaptureStdout("echo", "hi")
	if err != nil || out != "stdout-only" {
		t.Fatalf("runCaptureStdout = %q, %v; want stdout only, not the combined output", out, err)
	}
}

// TestExecRunner_CaptureStdoutIncludesStderrInError doesn't parse the command's stdout, but a
// failure with no stderr text at all makes a route/ip/ifconfig failure much harder to diagnose,
// so the trimmed stderr is folded into the error message.
func TestExecRunner_CaptureStdoutIncludesStderrInError(t *testing.T) {
	_, err := execRunner{}.CaptureStdout("sh", "-c", "echo out; echo something went wrong >&2; exit 1")
	if err == nil {
		t.Fatal("want an error from a nonzero exit")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Fatalf("err = %q, want it to include the command's stderr", err)
	}
}

func useCommandTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := commandTimeout
	commandTimeout = d
	t.Cleanup(func() { commandTimeout = old })
}

func TestExecRunner_HungCommandTimesOut(t *testing.T) {
	useCommandTimeout(t, 100*time.Millisecond)
	for name, call := range map[string]func() error{
		"Run":           func() error { return execRunner{}.Run("sleep", "10") },
		"Capture":       func() error { _, err := execRunner{}.Capture("sleep", "10"); return err },
		"CaptureStdout": func() error { _, err := execRunner{}.CaptureStdout("sleep", "10"); return err },
		"RunInput":      func() error { _, err := execRunner{}.RunInput("", "sleep", "10"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			err := call()
			if err == nil || time.Since(start) > 3*time.Second {
				t.Fatalf("%s = %v after %v; a hung tool must be killed at the timeout", name, err, time.Since(start))
			}
			if !strings.Contains(err.Error(), "sleep timed out after 100ms") {
				t.Fatalf("%s = %q; want it to name the tool and say it timed out, not a bare kill signal", name, err)
			}
		})
	}
}

func TestExecRunner_FailureWithinTheTimeoutIsNotCalledATimeout(t *testing.T) {
	err := execRunner{}.Run("sh", "-c", "exit 3")
	if err == nil || strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Run = %v; want a plain exit error", err)
	}
}

func TestExecRunner_RunKeepsToolOutputOffStdout(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	runErr := execRunner{}.Run("sh", "-c", "echo route noise")
	os.Stdout = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)

	if runErr != nil || len(out) != 0 {
		t.Fatalf("Run = %v, stdout %q; tool output must not share the process's stdout", runErr, out)
	}
}

func TestExecRunner_RunErrorCarriesToolOutput(t *testing.T) {
	err := execRunner{}.Run("sh", "-c", "echo route: writing to routing socket: File exists >&2; exit 1")
	if err == nil || !strings.Contains(err.Error(), "File exists") {
		t.Fatalf("Run = %v; want the tool's own message in the error", err)
	}
}
