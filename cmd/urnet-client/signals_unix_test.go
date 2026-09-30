//go:build darwin || linux

package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

func TestShutdownSignalsCancelContext(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, stop := notifyShutdown(context.Background())
			defer stop()

			if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
				t.Fatal(err)
			}

			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatalf("%v did not cancel the run context; route and DNS cleanup would be skipped", sig)
			}
		})
	}
}

func TestShutdownSignalsLeaveSIGQUITForGoroutineDumps(t *testing.T) {
	if slices.Contains(shutdownSignals, os.Signal(syscall.SIGQUIT)) {
		t.Fatal("SIGQUIT must keep its default: a goroutine dump of a hung process")
	}
}

const (
	signalHelperEnv     = "URNET_TEST_SIGNAL_HELPER"
	brokenPipeHelperEnv = "URNET_TEST_BROKEN_PIPE_HELPER"
)

// TestMain lets TestSecondSignalGetsDefaultDisposition run this binary as a process that
// catches its first signal and then hangs, like a run stuck in cleanup.
func TestMain(m *testing.M) {
	if c := os.Getenv(brokenPipeHelperEnv); c != "" {
		ignoreBrokenPipeFor(docopt.Opts{c: true})
		for i := 0; i < 3; i++ {
			logx.Warn("cleanup step %d\n", i)
			logx.Info("cleanup step %d\n", i)
		}
		os.Exit(0)
	}
	if os.Getenv(signalHelperEnv) == "1" {
		ctx, stop := notifyShutdown(context.Background())
		defer stop()
		_, _ = os.Stdout.WriteString("ready\n")
		<-ctx.Done()
		_, _ = os.Stdout.WriteString("cancelled\n")
		select {}
	}
	os.Exit(m.Run())
}

func TestSecondSignalGetsDefaultDisposition(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), signalHelperEnv+"=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	lines := bufio.NewScanner(out)
	expect := func(want string) {
		t.Helper()
		if !lines.Scan() || lines.Text() != want {
			t.Fatalf("helper said %q, want %q", lines.Text(), want)
		}
	}
	expect("ready")
	_ = cmd.Process.Signal(syscall.SIGTERM)
	expect("cancelled")

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	// Resent because the handler is released just after the context is cancelled.
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case err := <-done:
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGTERM {
				t.Fatalf("helper ended with %v; want killed by the second SIGTERM", err)
			}
			return
		case <-tick.C:
			_ = cmd.Process.Signal(syscall.SIGTERM)
		case <-timeout:
			t.Fatal("a second SIGTERM did not end a process stuck after the first")
		}
	}
}

// runWithDeadOutput runs the helper for command with stdout and stderr on a pipe nobody reads.
func runWithDeadOutput(t *testing.T, command string) error {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), brokenPipeHelperEnv+"="+command)
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	return cmd.Wait()
}

func TestCommandsThatChangeSystemStateSurviveADeadLogReader(t *testing.T) {
	for _, c := range []string{"vpn", "quick-connect", "recover"} {
		t.Run(c, func(t *testing.T) {
			if err := runWithDeadOutput(t, c); err != nil {
				t.Fatalf("%s ended with %v on a broken log pipe; DNS cleanup would stop half way", c, err)
			}
		})
	}
}

func TestOtherCommandsKeepTheDefaultBrokenPipeExit(t *testing.T) {
	err := runWithDeadOutput(t, "locations")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGPIPE {
		t.Fatalf("locations ended with %v; want the default SIGPIPE exit, like `| head`", err)
	}
}
