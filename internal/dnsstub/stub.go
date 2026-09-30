// Package dnsstub is the local DNS forwarder the system resolver points at while
// --default_route is active, so every lookup follows one policy and one upstream path.
package dnsstub

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/miekg/dns"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

const (
	defaultTimeout = 2 * time.Second
	pinnedTTL      = 60
)

// Options configures Start.
type Options struct {
	Listen       netip.AddrPort   // UDP and TCP address, normally <TUN address>:53
	Upstreams    []netip.AddrPort // tried in order
	Control      func(network, address string, c syscall.RawConn) error
	BindUpstream bool          // pass every upstream socket through Control from the start
	Timeout      time.Duration // per upstream exchange; zero means defaultTimeout
	FilterAAAA   bool          // answer AAAA with an empty NOERROR while IPv6 is blocked
	// Pinned answers A queries for the control-plane hosts with the addresses resolved at
	// startup, so reconnecting never depends on the tunnel it is trying to rebuild.
	Pinned     map[string][]netip.Addr
	LocalAddrs func() ([]net.Addr, error) // nil means net.InterfaceAddrs
}

// Server is a running stub. Close stops it.
type Server struct {
	opts   Options
	pinned map[string][]netip.Addr
	addr   netip.AddrPort
	bound  atomic.Bool
	udp    *dns.Server
	tcp    *dns.Server
}

// Start binds UDP and TCP on opts.Listen before it returns, so a port conflict fails startup.
func Start(opts Options) (*Server, error) {
	if len(opts.Upstreams) == 0 {
		return nil, errors.New("dnsstub: no upstream servers")
	}
	if opts.Control == nil {
		return nil, errors.New("dnsstub: Control is required to pin upstream sockets to the tunnel")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.LocalAddrs == nil {
		opts.LocalAddrs = net.InterfaceAddrs
	}
	pc, ln, err := listenUDPTCP(opts.Listen)
	if err != nil {
		return nil, fmt.Errorf("dnsstub: listen on %s: %w", opts.Listen, err)
	}
	s := &Server{opts: opts, pinned: normalizePinned(opts.Pinned), addr: unmapped(pc.LocalAddr().(*net.UDPAddr).AddrPort())}
	s.bound.Store(opts.BindUpstream)
	s.udp = &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(s.serve)}
	s.tcp = &dns.Server{Listener: ln, Handler: dns.HandlerFunc(s.serve)}
	if err := s.activate(); err != nil {
		_ = pc.Close()
		_ = ln.Close()
		return nil, fmt.Errorf("dnsstub: serve on %s: %w", opts.Listen, err)
	}
	logx.Info("DNS stub listening on %s (udp+tcp), upstreams %v\n", s.addr, opts.Upstreams)
	return s, nil
}

// Addr is the bound address; with port 0 in Options it carries the port the kernel chose.
func (s *Server) Addr() netip.AddrPort { return s.addr }

// BindUpstream pins every later upstream socket to the tunnel (the dns_bootstrap=cache switch).
func (s *Server) BindUpstream() { s.bound.Store(true) }

// Close stops both listeners and waits up to a second for in-flight queries.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return errors.Join(s.udp.ShutdownContext(ctx), s.tcp.ShutdownContext(ctx))
}

func (s *Server) activate() error {
	ready := make(chan struct{}, 2)
	failed := make(chan error, 2)
	for _, srv := range []*dns.Server{s.udp, s.tcp} {
		srv.NotifyStartedFunc = func() { ready <- struct{}{} }
		go func() {
			if err := srv.ActivateAndServe(); err != nil {
				failed <- err
			}
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case err := <-failed:
			return err
		}
	}
	return nil
}

func (s *Server) serve(w dns.ResponseWriter, req *dns.Msg) {
	if !sourceAllowed(addrIP(w.RemoteAddr()), s.opts.LocalAddrs) {
		logx.Debug("dnsstub: refused query from %s\n", w.RemoteAddr())
		_ = w.WriteMsg(new(dns.Msg).SetRcode(req, dns.RcodeRefused))
		return
	}
	_, overTCP := w.RemoteAddr().(*net.TCPAddr)
	resp := s.answer(req, overTCP)
	if !overTCP {
		resp.Truncate(udpLimit(req))
	}
	if err := w.WriteMsg(resp); err != nil {
		logx.Debug("dnsstub: write reply: %v\n", err)
	}
}

func (s *Server) answer(req *dns.Msg, overTCP bool) *dns.Msg {
	if resp := s.local(req); resp != nil {
		return resp
	}
	resp, err := s.forward(req, overTCP)
	if err != nil {
		logx.Debug("dnsstub: no upstream answered: %v\n", err)
		return new(dns.Msg).SetRcode(req, dns.RcodeServerFailure)
	}
	return resp
}

// local answers without asking upstream: pinned control-plane names, and an empty AAAA
// answer while IPv6 is blocked so apps go straight to IPv4 instead of waiting on a v6 attempt.
func (s *Server) local(req *dns.Msg) *dns.Msg {
	if len(req.Question) != 1 || req.Question[0].Qclass != dns.ClassINET {
		return nil
	}
	q := req.Question[0]
	ips, pinned := s.pinned[dns.CanonicalName(q.Name)]
	switch {
	case q.Qtype == dns.TypeAAAA && (pinned || s.opts.FilterAAAA):
		return localReply(req)
	case q.Qtype == dns.TypeA && pinned:
		m := localReply(req)
		for _, ip := range ips {
			m.Answer = append(m.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: pinnedTTL},
				A:   ip.AsSlice(),
			})
		}
		return m
	}
	return nil
}

func localReply(req *dns.Msg) *dns.Msg {
	m := new(dns.Msg).SetReply(req)
	m.RecursionAvailable = true
	return m
}

// forward keeps the client's transport: the TCP retry after a truncated UDP answer has to
// reach the upstream over TCP as well, or it would come back truncated again.
func (s *Server) forward(req *dns.Msg, overTCP bool) (*dns.Msg, error) {
	c := s.client(overTCP)
	var errs []error
	for _, up := range s.opts.Upstreams {
		resp, _, err := c.Exchange(req, up.String())
		if err == nil {
			return resp, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", up, err))
	}
	return nil, errors.Join(errs...)
}

func (s *Server) client(overTCP bool) *dns.Client {
	c := &dns.Client{Net: "udp", UDPSize: dns.MaxMsgSize, Timeout: s.opts.Timeout}
	if overTCP {
		c.Net = "tcp"
	}
	if s.bound.Load() {
		c.Dialer = &net.Dialer{Timeout: s.opts.Timeout, Control: s.opts.Control}
	}
	return c
}

func udpLimit(req *dns.Msg) int {
	if opt := req.IsEdns0(); opt != nil {
		return int(opt.UDPSize())
	}
	return dns.MinMsgSize
}

// listenUDPTCP binds UDP first, then TCP on the same port. With port 0 it retries, because
// the port the kernel picked for UDP can already be taken for TCP.
func listenUDPTCP(addr netip.AddrPort) (net.PacketConn, net.Listener, error) {
	for attempt := 1; ; attempt++ {
		pc, err := net.ListenPacket("udp", addr.String())
		if err != nil {
			return nil, nil, err
		}
		port := pc.LocalAddr().(*net.UDPAddr).AddrPort().Port()
		ln, err := net.Listen("tcp", netip.AddrPortFrom(addr.Addr(), port).String())
		if err == nil {
			return pc, ln, nil
		}
		_ = pc.Close()
		if addr.Port() != 0 || attempt == 5 {
			return nil, nil, err
		}
	}
}

// normalizePinned keeps IPv4 only: the control-plane bypass routes are IPv4 host routes.
func normalizePinned(in map[string][]netip.Addr) map[string][]netip.Addr {
	out := make(map[string][]netip.Addr, len(in))
	for name, ips := range in {
		var v4 []netip.Addr
		for _, ip := range ips {
			if ip = ip.Unmap(); ip.Is4() {
				v4 = append(v4, ip)
			}
		}
		if len(v4) > 0 {
			out[dns.CanonicalName(name)] = v4
		}
	}
	return out
}

// sourceAllowed keeps the stub from serving other hosts that can reach the TUN address,
// for example through IP forwarding on a router.
func sourceAllowed(ip netip.Addr, localAddrs func() ([]net.Addr, error)) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() {
		return true
	}
	addrs, err := localAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if la, ok := netip.AddrFromSlice(n.IP); ok && la.Unmap() == ip {
			return true
		}
	}
	return false
}

func addrIP(a net.Addr) netip.Addr {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.AddrPort().Addr()
	case *net.TCPAddr:
		return v.AddrPort().Addr()
	}
	return netip.Addr{}
}

func unmapped(ap netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
}
