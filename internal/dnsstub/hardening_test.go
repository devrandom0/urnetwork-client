package dnsstub

import (
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// blackHole reads queries and never answers, like an upstream behind a dead tunnel.
func blackHole(t *testing.T) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			if _, _, err := pc.ReadFrom(buf); err != nil {
				return
			}
		}
	}()
	return unmapped(pc.LocalAddr().(*net.UDPAddr).AddrPort())
}

func TestStub_ServfailWithinBudgetAcrossAllUpstreams(t *testing.T) {
	s := startStub(t, Options{Upstreams: []netip.AddrPort{blackHole(t), blackHole(t), blackHole(t)}})

	start := time.Now()
	r, err := exchange(s, "udp", "example.test", dns.TypeA, 5*time.Second)
	elapsed := time.Since(start)

	if err != nil || r.Rcode != dns.RcodeServerFailure {
		t.Fatalf("reply = %v, %v; want SERVFAIL", r, err)
	}
	if elapsed > 1300*time.Millisecond {
		t.Fatalf("SERVFAIL after %v; mDNSResponder falls back to the LAN resolver when the stub is this slow", elapsed)
	}
}

func TestStub_BudgetLeavesTimeForLaterUpstreams(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.5"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{blackHole(t), up.addr}})

	if r := query(t, s, "udp", "example.test", dns.TypeA); len(r.Answer) != 1 {
		t.Fatalf("answer = %v; a silent first upstream must not use up the whole budget", r.Answer)
	}
}

type fakeWriter struct {
	remote  net.Addr
	written []*dns.Msg
}

func (w *fakeWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("10.255.0.2"), Port: 53}
}
func (w *fakeWriter) RemoteAddr() net.Addr      { return w.remote }
func (w *fakeWriter) WriteMsg(m *dns.Msg) error { w.written = append(w.written, m); return nil }
func (w *fakeWriter) Write([]byte) (int, error) { return 0, errors.New("unused") }
func (w *fakeWriter) Close() error              { return nil }
func (w *fakeWriter) TsigStatus() error         { return nil }
func (w *fakeWriter) TsigTimersOnly(bool)       {}
func (w *fakeWriter) Hijack()                   {}
func (w *fakeWriter) Network() string           { return "udp" }

func TestStub_DropsDisallowedUDPSourceSilently(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.1"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, LocalAddrs: func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("10.255.0.2"), Mask: net.CIDRMask(24, 32)}}, nil
	}})
	w := &fakeWriter{remote: &net.UDPAddr{IP: net.ParseIP("10.255.0.7"), Port: 5353}}

	s.serve(w, new(dns.Msg).SetQuestion("example.test.", dns.TypeA))

	if len(w.written) != 0 {
		t.Fatalf("stub replied %v to another host; a reply confirms the stub and can be spoofed into reflection", w.written)
	}
	if got := up.networks(); len(got) != 0 {
		t.Fatalf("upstream saw %v for a disallowed source", got)
	}
}

func loopbackListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func dialAndWaitClose(t *testing.T, addr net.Addr) {
	t.Helper()
	c, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil || isTimeout(err) {
		t.Fatalf("read = %v; want the connection closed at accept", err)
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func TestGuardedListener_ClosesDisallowedAtAccept(t *testing.T) {
	var allow atomic.Bool
	ln := newGuardedListener(loopbackListener(t), func(netip.Addr) bool { return allow.Load() }, 4)
	defer func() { _ = ln.Close() }()
	accepted := make(chan net.Conn, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			accepted <- c
		}
	}()

	dialAndWaitClose(t, ln.Addr())
	allow.Store(true)
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	select {
	case got := <-accepted:
		_ = got.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("allowed connection never accepted")
	}
}

func TestGuardedListener_CapsConcurrentConnections(t *testing.T) {
	ln := newGuardedListener(loopbackListener(t), func(netip.Addr) bool { return true }, 2)
	defer func() { _ = ln.Close() }()
	var mu sync.Mutex
	var conns []net.Conn
	accepted := make(chan struct{}, 3)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
			accepted <- struct{}{}
		}
	}()
	for range 3 {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
	}
	for range 2 {
		<-accepted
	}
	select {
	case <-accepted:
		t.Fatal("third connection accepted over the cap of 2")
	case <-time.After(200 * time.Millisecond):
	}

	mu.Lock()
	_ = conns[0].Close()
	_ = conns[0].Close()
	mu.Unlock()

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("closing a connection did not free a slot")
	}
}

func TestGuardedListener_CloseUnblocksAcceptAtCap(t *testing.T) {
	ln := newGuardedListener(loopbackListener(t), func(netip.Addr) bool { return true }, 1)
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	first, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	done := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		done <- err
	}()

	_ = ln.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Accept after Close returned a connection")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept stayed blocked after Close; Server.Close would hang")
	}
}

func TestLocalAddrCache_RefreshesAtMostOncePerTTL(t *testing.T) {
	var calls atomic.Int32
	now := time.Unix(1000, 0)
	c := &localAddrCache{
		list: func() ([]net.Addr, error) { calls.Add(1); return nil, nil },
		now:  func() time.Time { return now },
		ttl:  3 * time.Second,
	}

	for range 5 {
		_, _ = c.get()
	}
	if calls.Load() != 1 {
		t.Fatalf("interface list read %d times within the TTL, want 1", calls.Load())
	}
	now = now.Add(3 * time.Second)
	_, _ = c.get()
	if calls.Load() != 2 {
		t.Fatalf("interface list read %d times, want a refresh after the TTL", calls.Load())
	}
}
