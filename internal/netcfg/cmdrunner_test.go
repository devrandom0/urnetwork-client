package netcfg

import (
	"errors"
	"strings"
	"sync"
	"testing"
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
}

func useFakeRunner(t *testing.T) *fakeRunner {
	t.Helper()
	f := &fakeRunner{results: map[string]fakeResult{}}
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
	f.mu.Unlock()
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
