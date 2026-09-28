package socks

import (
	"context"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

type scriptedListener struct {
	mu     sync.Mutex
	errs   []error
	conns  []net.Conn
	closed chan struct{}
	once   sync.Once
}

func newScriptedListener(errs []error, conns []net.Conn) *scriptedListener {
	return &scriptedListener{errs: errs, conns: conns, closed: make(chan struct{})}
}

func (l *scriptedListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if len(l.errs) > 0 {
		err := l.errs[0]
		l.errs = l.errs[1:]
		l.mu.Unlock()
		return nil, err
	}
	if len(l.conns) > 0 {
		c := l.conns[0]
		l.conns = l.conns[1:]
		l.mu.Unlock()
		return c, nil
	}
	l.mu.Unlock()
	<-l.closed
	return nil, net.ErrClosed
}

func (l *scriptedListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *scriptedListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func emfileErr() error {
	return &net.OpError{Op: "accept", Net: "tcp", Err: os.NewSyscallError("accept", syscall.EMFILE)}
}

func TestAcceptLoop_RetriesAfterTransientErrors(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c2.Close() }()
	ln := newScriptedListener([]error{emfileErr(), emfileErr(), emfileErr()}, []net.Conn{c1})
	handled := make(chan net.Conn, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		acceptLoop(context.Background(), ln, 10*time.Millisecond, func(c net.Conn) { handled <- c })
	}()

	select {
	case c := <-handled:
		_ = c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("acceptLoop gave up after transient accept errors")
	}
	_ = ln.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acceptLoop did not exit after the listener closed")
	}
}

func TestAcceptLoop_ExitsOnContextCancelWhileBackingOff(t *testing.T) {
	errs := make([]error, 20)
	for i := range errs {
		errs[i] = emfileErr()
	}
	ln := newScriptedListener(errs, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		acceptLoop(ctx, ln, time.Second, func(net.Conn) {})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acceptLoop ignored context cancellation")
	}
	_ = ln.Close()
}
