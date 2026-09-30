package session

import (
	"os"
	"os/signal"
	"syscall"
)

// ShutdownSignals end a run through context cancellation so cleanup runs; SIGHUP arrives
// when the controlling terminal closes. SIGQUIT keeps its default goroutine dump.
var ShutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

// watchSecondSignal is called once cleanup has started: the next shutdown signal closes
// the returned channel so cleanup can stop after restoring DNS.
func watchSecondSignal() (abort <-chan struct{}, stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, ShutdownSignals...)
	done := make(chan struct{})
	quit := make(chan struct{})
	go func() {
		select {
		case <-ch:
			close(done)
		case <-quit:
		}
	}()
	return done, func() { signal.Stop(ch); close(quit) }
}
