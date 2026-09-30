package dnsstub

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type upstream struct {
	addr netip.AddrPort
	mu   sync.Mutex
	nets []string
}

func (u *upstream) networks() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.nets...)
}

// startUpstream runs a fake resolver on loopback, UDP and TCP on one port.
func startUpstream(t *testing.T, h func(w dns.ResponseWriter, req *dns.Msg, overTCP bool)) *upstream {
	t.Helper()
	pc, ln, err := listenUDPTCP(netip.MustParseAddrPort("127.0.0.1:0"))
	if err != nil {
		t.Fatalf("upstream listen: %v", err)
	}
	u := &upstream{addr: unmapped(pc.LocalAddr().(*net.UDPAddr).AddrPort())}
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		_, overTCP := w.RemoteAddr().(*net.TCPAddr)
		u.mu.Lock()
		if overTCP {
			u.nets = append(u.nets, "tcp")
		} else {
			u.nets = append(u.nets, "udp")
		}
		u.mu.Unlock()
		h(w, req, overTCP)
	})
	for _, srv := range []*dns.Server{{PacketConn: pc, Handler: handler}, {Listener: ln, Handler: handler}} {
		started := make(chan struct{})
		srv.NotifyStartedFunc = func() { close(started) }
		go func() { _ = srv.ActivateAndServe() }()
		<-started
		t.Cleanup(func() { _ = srv.Shutdown() })
	}
	return u
}

func aRecord(name, ip string) dns.RR {
	return &dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP(ip).To4()}
}

func answerA(ip string) func(dns.ResponseWriter, *dns.Msg, bool) {
	return func(w dns.ResponseWriter, req *dns.Msg, _ bool) {
		m := new(dns.Msg).SetReply(req)
		if req.Question[0].Qtype == dns.TypeA {
			m.Answer = append(m.Answer, aRecord(req.Question[0].Name, ip))
		}
		_ = w.WriteMsg(m)
	}
}

func allowControl(string, string, syscall.RawConn) error { return nil }

func startStub(t *testing.T, opts Options) *Server {
	t.Helper()
	if !opts.Listen.IsValid() {
		opts.Listen = netip.MustParseAddrPort("127.0.0.1:0")
	}
	if opts.Control == nil {
		opts.Control = allowControl
	}
	s, err := Start(opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func exchange(s *Server, network, name string, qtype uint16, timeout time.Duration) (*dns.Msg, error) {
	m := new(dns.Msg).SetQuestion(dns.Fqdn(name), qtype)
	c := &dns.Client{Net: network, Timeout: timeout}
	r, _, err := c.Exchange(m, s.Addr().String())
	return r, err
}

func query(t *testing.T, s *Server, network, name string, qtype uint16) *dns.Msg {
	t.Helper()
	r, err := exchange(s, network, name, qtype, 3*time.Second)
	if err != nil {
		t.Fatalf("%s query for %s: %v", network, name, err)
	}
	return r
}

func closedUDPPort(t *testing.T) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ap := unmapped(pc.LocalAddr().(*net.UDPAddr).AddrPort())
	_ = pc.Close()
	return ap
}

type recordingControl struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (r *recordingControl) control(network, address string, _ syscall.RawConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, network+" "+address)
	return r.err
}

func (r *recordingControl) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func TestStub_ForwardsUDPQueryOverUDP(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.1"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}})

	r := query(t, s, "udp", "example.test", dns.TypeA)

	if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "192.0.2.1" {
		t.Fatalf("answer = %v; want the upstream's A record", r.Answer)
	}
	if got := up.networks(); len(got) != 1 || got[0] != "udp" {
		t.Fatalf("upstream saw %v; want one UDP query", got)
	}
}

func TestStub_ForwardsTCPQueryOverTCP(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.1"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}})

	r := query(t, s, "tcp", "example.test", dns.TypeA)

	if len(r.Answer) != 1 {
		t.Fatalf("answer = %v; want the upstream's A record", r.Answer)
	}
	if got := up.networks(); len(got) != 1 || got[0] != "tcp" {
		t.Fatalf("upstream saw %v; want one TCP query", got)
	}
}

func TestStub_PassesUpstreamTruncationSoClientRetriesOverTCP(t *testing.T) {
	up := startUpstream(t, func(w dns.ResponseWriter, req *dns.Msg, overTCP bool) {
		m := new(dns.Msg).SetReply(req)
		if overTCP {
			m.Answer = []dns.RR{aRecord(req.Question[0].Name, "192.0.2.9")}
		} else {
			m.Truncated = true
		}
		_ = w.WriteMsg(m)
	})
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}})

	if r := query(t, s, "udp", "big.test", dns.TypeA); !r.Truncated {
		t.Fatal("UDP reply lost the upstream TC bit; the client would never retry over TCP")
	}
	if r := query(t, s, "tcp", "big.test", dns.TypeA); r.Truncated || len(r.Answer) != 1 {
		t.Fatalf("TCP reply: truncated=%v answers=%d; want the full answer", r.Truncated, len(r.Answer))
	}
}

func TestStub_TruncatesToClientUDPBuffer(t *testing.T) {
	up := startUpstream(t, func(w dns.ResponseWriter, req *dns.Msg, _ bool) {
		m := new(dns.Msg).SetReply(req)
		for i := range 60 {
			m.Answer = append(m.Answer, aRecord(req.Question[0].Name, fmt.Sprintf("192.0.2.%d", i+1)))
		}
		_ = w.WriteMsg(m)
	})
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}})

	if r := query(t, s, "udp", "many.test", dns.TypeA); !r.Truncated || len(r.Answer) >= 60 {
		t.Fatalf("UDP reply: truncated=%v answers=%d; a reply over 512 bytes must be cut and flagged", r.Truncated, len(r.Answer))
	}
	if r := query(t, s, "tcp", "many.test", dns.TypeA); len(r.Answer) != 60 {
		t.Fatalf("TCP reply has %d answers, want 60", len(r.Answer))
	}
}

func TestStub_FallsBackToNextUpstream(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.2"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{closedUDPPort(t), up.addr}, Budget: 300 * time.Millisecond})

	if r := query(t, s, "udp", "example.test", dns.TypeA); len(r.Answer) != 1 {
		t.Fatalf("answer = %v; want the second upstream's record", r.Answer)
	}
}

func TestStub_ServfailWhenNoUpstreamAnswers(t *testing.T) {
	s := startStub(t, Options{Upstreams: []netip.AddrPort{closedUDPPort(t)}, Budget: 300 * time.Millisecond})

	if r := query(t, s, "udp", "example.test", dns.TypeA); r.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %s, want SERVFAIL", dns.RcodeToString[r.Rcode])
	}
}

func TestStart_FailsWhenAddressInUse(t *testing.T) {
	busy, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()
	addr := unmapped(busy.LocalAddr().(*net.UDPAddr).AddrPort())

	if _, err := Start(Options{Listen: addr, Upstreams: []netip.AddrPort{addr}, Control: allowControl}); err == nil {
		t.Fatal("Start succeeded on a busy port; startup must fail instead of running without the stub")
	}
}

func TestStart_RequiresUpstreamsAndControl(t *testing.T) {
	listen := netip.MustParseAddrPort("127.0.0.1:0")
	up := netip.MustParseAddrPort("192.0.2.53:53")
	if _, err := Start(Options{Listen: listen, Control: allowControl}); err == nil {
		t.Fatal("Start without upstreams succeeded")
	}
	if _, err := Start(Options{Listen: listen, Upstreams: []netip.AddrPort{up}}); err == nil || !strings.Contains(err.Error(), "Control") {
		t.Fatalf("Start without Control = %v; want an error, the stub could never pin its sockets", err)
	}
}

func TestStub_CloseStopsServing(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.1"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := exchange(s, "tcp", "example.test", dns.TypeA, 300*time.Millisecond); err == nil {
		t.Fatal("stub still answered over TCP after Close")
	}
}

func TestStub_BoundUpstreamGoesThroughControl(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.3"))
	rc := &recordingControl{}
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, Control: rc.control, BindUpstream: true})

	query(t, s, "udp", "example.test", dns.TypeA)
	query(t, s, "tcp", "example.test", dns.TypeA)

	want := []string{"udp4 " + up.addr.String(), "tcp4 " + up.addr.String()}
	if got := rc.seen(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Control saw %v; want %v", got, want)
	}
}

func TestStub_BoundUpstreamFailsClosedWhenBindFails(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.3"))
	rc := &recordingControl{err: errors.New("bind to VPN interface failed")}
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, Control: rc.control, BindUpstream: true})

	if r := query(t, s, "udp", "example.test", dns.TypeA); r.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %s, want SERVFAIL", dns.RcodeToString[r.Rcode])
	}
	if got := up.networks(); len(got) != 0 {
		t.Fatalf("upstream got %v although binding failed; the query left outside the tunnel", got)
	}
}

func TestStub_UnboundUpstreamSkipsControl(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.3"))
	rc := &recordingControl{err: errors.New("must not be called")}
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, Control: rc.control})

	if r := query(t, s, "udp", "example.test", dns.TypeA); len(r.Answer) != 1 {
		t.Fatalf("answer = %v; bypass mode must use normal routing", r.Answer)
	}
	if got := rc.seen(); len(got) != 0 {
		t.Fatalf("Control called %v in bypass mode", got)
	}
}

func TestStub_BindUpstreamAppliesToLaterQueries(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.3"))
	rc := &recordingControl{}
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, Control: rc.control})

	query(t, s, "udp", "before.test", dns.TypeA)
	s.BindUpstream()
	query(t, s, "udp", "after.test", dns.TypeA)

	if got := rc.seen(); len(got) != 1 {
		t.Fatalf("Control calls = %v; want exactly the query after BindUpstream", got)
	}
}
