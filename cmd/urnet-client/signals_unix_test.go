//go:build darwin || linux

package main

import (
	"context"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestShutdownSignalsCancelContext(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
			defer stop()

			if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
				t.Fatal(err)
			}

			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
				t.Fatalf("%v did not cancel the run context; deferred route and DNS cleanup would be skipped", sig)
			}
		})
	}
}
