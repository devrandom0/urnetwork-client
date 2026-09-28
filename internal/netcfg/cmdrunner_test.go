package netcfg

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakeResult struct {
	out string
	err error
}

// fakeRunner records every command and returns scripted results; safe for concurrent use.
type fakeRunner struct {
	mu      sync.Mutex
	calls   []string
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
