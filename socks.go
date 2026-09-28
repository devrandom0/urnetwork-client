package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

const (
	socksVersion5 = 0x05

	socksMethodNoAuth       = 0x00
	socksMethodUserPass     = 0x02
	socksMethodNoAcceptable = 0xFF

	socksCmdConnect      = 0x01
	socksCmdUDPAssociate = 0x03

	socksATYPIPv4   = 0x01
	socksATYPDomain = 0x03
	socksATYPIPv6   = 0x04

	socksRepSucceeded            = 0x00
	socksRepGeneralFailure       = 0x01
	socksRepNetUnreachable       = 0x03
	socksRepHostUnreachable      = 0x04
	socksRepConnRefused          = 0x05
	socksRepCmdNotSupported      = 0x07
	socksRepAddrTypeNotSupported = 0x08

	defaultSocksHandshakeTimeout = 10 * time.Second
)

var errSocksBadATYP = errors.New("unsupported SOCKS address type")

// SocksOptions configures StartSocks5.
type SocksOptions struct {
	ListenAddr       string
	BindIf           string // interface that VPN-routed traffic must leave through; empty means system routing
	Debug            bool
	AllowDomains     []string
	ExcludeDomains   []string
	DNSServers       []string      // first entry replaces the system resolver for hostname lookups
	HandshakeTimeout time.Duration // zero means defaultSocksHandshakeTimeout
	Auth             SocksAuth
	MaxConns         int           // concurrent client connections; zero means defaultSocksMaxConns
	UDPIdleTimeout   time.Duration // zero means defaultSocksUDPIdleTimeout
}

// StartSocks5 starts a SOCKS5 proxy and returns a stop function.
func StartSocks5(ctx context.Context, opts SocksOptions) (func() error, error) {
	if err := opts.Auth.validate(); err != nil {
		return nil, err
	}
	if opts.HandshakeTimeout <= 0 {
		opts.HandshakeTimeout = defaultSocksHandshakeTimeout
	}
	if opts.MaxConns <= 0 {
		opts.MaxConns = defaultSocksMaxConns
	}
	if opts.UDPIdleTimeout <= 0 {
		opts.UDPIdleTimeout = defaultSocksUDPIdleTimeout
	}
	srv := &socksServer{
		opts:  opts,
		slots: make(chan struct{}, opts.MaxConns),
		warns: newLogRateLimiter(socksWarnInterval),
	}
	srv.sysResolver = newSocksResolver(opts.DNSServers, nil)
	srv.vpnResolver = srv.sysResolver
	if opts.BindIf != "" {
		srv.vpnResolver = newSocksResolver(opts.DNSServers, srv.bindControl)
	}
	ln, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, err
	}
	if socksExposedWithoutAuth(ln.Addr(), opts.Auth) {
		logx.Warn("SOCKS5 on %s accepts connections from other hosts WITHOUT authentication; anyone who can reach this port can use your VPN. Set --socks_user and URNETWORK_SOCKS_PASS, or bind 127.0.0.1\n", ln.Addr())
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		acceptLoop(ctx, ln, time.Second, func(conn net.Conn) { srv.admit(ctx, conn) })
	}()
	stop := func() error { _ = ln.Close(); <-done; return nil }
	return stop, nil
}

func socksExposedWithoutAuth(addr net.Addr, auth SocksAuth) bool {
	if auth.Enabled() {
		return false
	}
	ip := addrIP(addr)
	return ip == nil || !ip.IsLoopback()
}

type socksServer struct {
	opts        SocksOptions
	vpnResolver *net.Resolver // DNS bound to BindIf, for names routed through the VPN
	sysResolver *net.Resolver // DNS for names that bypass the VPN
	slots       chan struct{} // one token per live client connection
	warns       *logRateLimiter
}

// admit serves conn only while under MaxConns; over the cap it is closed before any
// handshake so a flood cannot exhaust goroutines and file descriptors.
func (s *socksServer) admit(ctx context.Context, conn net.Conn) {
	select {
	case s.slots <- struct{}{}:
		go func() {
			defer func() { <-s.slots }()
			s.handleConn(ctx, conn)
		}()
	default:
		_ = conn.Close()
		if s.warns.allow("conn-cap", time.Now()) {
			logx.Warn("socks: %d concurrent connections reached; dropping new connections\n", s.opts.MaxConns)
		}
	}
}

// acceptLoop keeps serving through transient Accept errors such as EMFILE and only
// stops when the listener is closed or ctx ends.
func acceptLoop(ctx context.Context, ln net.Listener, maxBackoff time.Duration, handle func(net.Conn)) {
	const minBackoff = 5 * time.Millisecond
	var backoff time.Duration
	for {
		conn, err := ln.Accept()
		if err == nil {
			backoff = 0
			handle(conn)
			continue
		}
		if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
			return
		}
		switch {
		case backoff == 0:
			backoff = minBackoff
		case backoff < maxBackoff:
			backoff = min(backoff*2, maxBackoff)
		}
		logx.Warn("socks accept failed: %v; retrying in %s\n", err, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

// newSocksResolver returns a resolver whose DNS sockets go through control. With
// dnsServers set, the first entry replaces the system resolver's servers. Any control
// error fails the lookup; there is deliberately no fallback to an unbound socket.
func newSocksResolver(dnsServers []string, control func(network, address string, c syscall.RawConn) error) *net.Resolver {
	if len(dnsServers) == 0 && control == nil {
		return net.DefaultResolver
	}
	var fixed string
	if len(dnsServers) > 0 {
		fixed = dnsServers[0]
		if _, _, err := net.SplitHostPort(fixed); err != nil {
			fixed = net.JoinHostPort(fixed, "53")
		}
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if fixed != "" {
				address = fixed
			}
			d := net.Dialer{Timeout: 5 * time.Second, Control: control}
			return d.DialContext(ctx, network, address)
		},
	}
}

func (s *socksServer) handleConn(ctx context.Context, c net.Conn) {
	defer func() { _ = c.Close() }()
	// Bounded handshake so idle clients cannot pin goroutines; each session clears it.
	_ = c.SetDeadline(time.Now().Add(s.opts.HandshakeTimeout))

	if err := s.negotiate(c); err != nil {
		if s.opts.Debug {
			fmt.Printf("[socks] handshake from %s failed: %v\n", c.RemoteAddr(), err)
		}
		return
	}
	hdr := make([]byte, 3)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return
	}
	if hdr[0] != socksVersion5 {
		_ = writeSocksReply(c, socksRepGeneralFailure, nil)
		return
	}
	dst, err := readSocksAddr(c)
	if err != nil {
		if errors.Is(err, errSocksBadATYP) {
			_ = writeSocksReply(c, socksRepAddrTypeNotSupported, nil)
		}
		return
	}
	switch hdr[1] {
	case socksCmdConnect:
		s.handleConnect(ctx, c, dst)
	case socksCmdUDPAssociate:
		s.runUDPAssociate(ctx, c, dst)
	default:
		_ = writeSocksReply(c, socksRepCmdNotSupported, nil)
	}
}

func (s *socksServer) negotiate(c net.Conn) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(c, head); err != nil {
		return err
	}
	if head[0] != socksVersion5 {
		return fmt.Errorf("unsupported SOCKS version %d", head[0])
	}
	offered := make([]byte, int(head[1]))
	if _, err := io.ReadFull(c, offered); err != nil {
		return err
	}
	method := selectSocksMethod(offered, s.opts.Auth.Enabled())
	if _, err := c.Write([]byte{socksVersion5, method}); err != nil {
		return err
	}
	switch method {
	case socksMethodNoAuth:
		return nil
	case socksMethodUserPass:
		err := socksUserPassAuth(c, s.opts.Auth, socksAuthFailureDelay)
		if errors.Is(err, errSocksAuthFailed) && s.warns.allow("auth:"+addrIP(c.RemoteAddr()).String(), time.Now()) {
			logx.Warn("socks: rejected credentials from %s (further failures from this address are not logged for %s)\n", c.RemoteAddr(), socksWarnInterval)
		}
		return err
	default:
		return errors.New("client offered no acceptable authentication method")
	}
}

type socksAddr struct {
	atyp byte
	host string // IP literal or domain name
	port int
}

func (a socksAddr) String() string { return net.JoinHostPort(a.host, strconv.Itoa(a.port)) }

func readSocksAddr(r io.Reader) (socksAddr, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(r, atyp[:]); err != nil {
		return socksAddr{}, err
	}
	var host string
	switch atyp[0] {
	case socksATYPIPv4, socksATYPIPv6:
		size := net.IPv4len
		if atyp[0] == socksATYPIPv6 {
			size = net.IPv6len
		}
		b := make([]byte, size)
		if _, err := io.ReadFull(r, b); err != nil {
			return socksAddr{}, err
		}
		host = net.IP(b).String()
	case socksATYPDomain:
		var l [1]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return socksAddr{}, err
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(r, b); err != nil {
			return socksAddr{}, err
		}
		host = string(b)
	default:
		return socksAddr{atyp: atyp[0]}, errSocksBadATYP
	}
	var p [2]byte
	if _, err := io.ReadFull(r, p[:]); err != nil {
		return socksAddr{}, err
	}
	return socksAddr{atyp: atyp[0], host: host, port: int(binary.BigEndian.Uint16(p[:]))}, nil
}

func (s *socksServer) handleConnect(ctx context.Context, c net.Conn, dst socksAddr) {
	var domain string
	if dst.atyp == socksATYPDomain {
		domain = strings.ToLower(dst.host)
	}
	useVPN := s.routeViaVPN(domain)
	ip := net.ParseIP(dst.host)
	if domain != "" {
		if ip = s.resolve(ctx, dst.host, useVPN); ip == nil {
			_ = writeSocksReply(c, socksRepHostUnreachable, nil)
			return
		}
	}
	target := net.JoinHostPort(ip.String(), strconv.Itoa(dst.port))
	if s.opts.Debug {
		fmt.Printf("[socks] CONNECT %s (ip=%s) bindIf=%s useVPN=%v\n", dst, ip, s.opts.BindIf, useVPN)
	}
	d := net.Dialer{Timeout: 30 * time.Second}
	if useVPN && s.opts.BindIf != "" {
		d.Control = s.bindControl
	}
	rc, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		rep := dialErrorReply(err)
		if errors.Is(err, errBindInterface) {
			logx.Warn("socks: %v; refusing CONNECT instead of sending it outside the VPN\n", withoutDialTarget(err))
			logx.Debug("socks: refused CONNECT to %s\n", dst)
		}
		if s.opts.Debug {
			fmt.Printf("[socks] dial error to %s: %v (rep=%d)\n", target, err, rep)
		}
		_ = writeSocksReply(c, rep, nil)
		return
	}
	defer func() { _ = rc.Close() }()
	if err := writeSocksReply(c, socksRepSucceeded, rc.LocalAddr()); err != nil {
		return
	}
	_ = c.SetDeadline(time.Time{})
	var wg sync.WaitGroup
	wg.Add(2)
	go relayTCP(rc, c, &wg)
	go relayTCP(c, rc, &wg)
	wg.Wait()
}

// withoutDialTarget strips the destination that net.OpError adds, so WARN logs do not record
// which hosts SOCKS clients visit.
func withoutDialTarget(err error) error {
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Err != nil {
		return oe.Err
	}
	return err
}

func relayTCP(dst, src net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	if tc, ok := dst.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}

func (s *socksServer) bindControl(network, _ string, rc syscall.RawConn) error {
	var bindErr error
	if err := rc.Control(func(fd uintptr) { bindErr = bindFDToInterface(int(fd), network, s.opts.BindIf) }); err != nil {
		return err
	}
	return bindErr
}

// resolve prefers IPv4 because the tunnel drops IPv6 unless --enable_ipv6 is set.
func (s *socksServer) resolve(ctx context.Context, host string, viaVPN bool) net.IP {
	r := s.sysResolver
	if viaVPN {
		r = s.vpnResolver
	}
	addrs, _ := r.LookupIP(ctx, "ip", host)
	var pick net.IP
	for _, ip := range addrs {
		if ip.To4() != nil {
			return ip
		}
		if pick == nil {
			pick = ip
		}
	}
	return pick
}

func (s *socksServer) routeViaVPN(domain string) bool {
	if len(s.opts.AllowDomains) > 0 && (domain == "" || !domainMatches(domain, s.opts.AllowDomains)) {
		return false
	}
	return domain == "" || !domainMatches(domain, s.opts.ExcludeDomains)
}

func dialErrorReply(err error) byte {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return socksRepHostUnreachable
	}
	var se syscall.Errno
	if errors.As(err, &se) {
		switch se {
		case syscall.ECONNREFUSED:
			return socksRepConnRefused
		case syscall.ENETUNREACH:
			return socksRepNetUnreachable
		case syscall.EHOSTUNREACH, syscall.ETIMEDOUT:
			return socksRepHostUnreachable
		}
	}
	return socksRepGeneralFailure
}

func writeSocksReply(w io.Writer, rep byte, bindAddr net.Addr) error {
	ip, port := net.IPv4zero, 0
	switch a := bindAddr.(type) {
	case *net.TCPAddr:
		ip, port = a.IP, a.Port
	case *net.UDPAddr:
		ip, port = a.IP, a.Port
	}
	resp := appendSocksIP([]byte{socksVersion5, rep, 0x00}, ip)
	resp = binary.BigEndian.AppendUint16(resp, uint16(port))
	_, err := w.Write(resp)
	return err
}

func appendSocksIP(b []byte, ip net.IP) []byte {
	if v4 := ip.To4(); v4 != nil {
		return append(append(b, socksATYPIPv4), v4...)
	}
	if v6 := ip.To16(); v6 != nil {
		return append(append(b, socksATYPIPv6), v6...)
	}
	return append(b, socksATYPIPv4, 0, 0, 0, 0)
}

// domainMatches checks if host matches any suffix in patterns (case-insensitive).
func domainMatches(host string, patterns []string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(p, ".")))
		if p == "" {
			continue
		}
		if h == p || strings.HasSuffix(h, "."+p) {
			return true
		}
	}
	return false
}

// udpAssociation is the per-ASSOCIATE state shared by the relay goroutines.
type udpAssociation struct {
	clientIP   net.IP
	client     atomic.Pointer[net.UDPAddr] // set once, never replaced
	peers      udpPeerSet
	lastActive atomic.Int64 // unix nanos of the last relayed datagram in either direction
}

func (a *udpAssociation) touch() { a.lastActive.Store(time.Now().UnixNano()) }

// closeWhenIdle closes ctrl, which tears the association down, once no datagram has been
// relayed for timeout. It returns when done is closed.
func (a *udpAssociation) closeWhenIdle(ctrl net.Conn, timeout time.Duration, done <-chan struct{}) {
	check := time.NewTicker(max(timeout/4, 10*time.Millisecond))
	defer check.Stop()
	for {
		select {
		case <-done:
			return
		case <-check.C:
			if time.Since(time.Unix(0, a.lastActive.Load())) >= timeout {
				_ = ctrl.Close()
				return
			}
		}
	}
}

// maxUDPPeersPerAssociation bounds memory per association. Evicting an arbitrary entry when
// full is acceptable because a legitimate client talks to few peers at once.
const maxUDPPeersPerAssociation = 1024

// udpPeerSet records the destinations an association has sent to; only they may reply.
type udpPeerSet struct {
	mu sync.Mutex
	m  map[netip.AddrPort]struct{}
}

func udpAddrPort(ip net.IP, port int) (netip.AddrPort, bool) {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(a.Unmap(), uint16(port)), true
}

func (p *udpPeerSet) add(ip net.IP, port int) {
	key, ok := udpAddrPort(ip, port)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = make(map[netip.AddrPort]struct{})
	}
	if _, seen := p.m[key]; !seen && len(p.m) >= maxUDPPeersPerAssociation {
		for k := range p.m {
			delete(p.m, k)
			break
		}
	}
	p.m[key] = struct{}{}
}

func (p *udpPeerSet) has(addr *net.UDPAddr) bool {
	key, ok := udpAddrPort(addr.IP, addr.Port)
	if !ok {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, found := p.m[key]
	return found
}

// runUDPAssociate implements SOCKS5 UDP ASSOCIATE for a single TCP control connection.
// req is the DST.ADDR/DST.PORT of the request, which RFC 1928 lets the client use to declare
// the port it will send from.
func (s *socksServer) runUDPAssociate(ctx context.Context, ctrl net.Conn, req socksAddr) {
	var (
		wg    sync.WaitGroup
		conns []net.PacketConn
	)
	defer func() {
		for _, pc := range conns {
			_ = pc.Close()
		}
		wg.Wait()
	}()
	refuse := func(format string, args ...any) {
		logx.Warn("socks udp: "+format, args...)
		_ = writeSocksReply(ctrl, socksRepGeneralFailure, nil)
	}

	// Listen where the client reached us, so LAN clients get a reachable relay address.
	relayAddr := net.JoinHostPort(addrIP(ctrl.LocalAddr()).String(), "0")
	pcClient, err := net.ListenPacket("udp", relayAddr)
	if err != nil {
		refuse("relay listen on %s: %v\n", relayAddr, err)
		return
	}
	conns = append(conns, pcClient)

	var pcVPN net.PacketConn
	if s.opts.BindIf != "" {
		lc := net.ListenConfig{Control: s.bindControl}
		if pcVPN, err = lc.ListenPacket(ctx, "udp4", "0.0.0.0:0"); err != nil {
			refuse("%v; refusing UDP ASSOCIATE instead of sending it outside the VPN\n", err)
			return
		}
		conns = append(conns, pcVPN)
	}
	pcSys, err := net.ListenPacket("udp", ":0")
	if err != nil {
		refuse("system socket: %v\n", err)
		return
	}
	conns = append(conns, pcSys)

	if err := writeSocksReply(ctrl, socksRepSucceeded, pcClient.LocalAddr()); err != nil {
		return
	}
	_ = ctrl.SetDeadline(time.Time{})
	if s.opts.Debug {
		fmt.Printf("[socks] UDP ASSOCIATE relay %s bindIf=%s\n", pcClient.LocalAddr(), s.opts.BindIf)
	}

	assoc := &udpAssociation{clientIP: addrIP(ctrl.RemoteAddr())}
	assoc.touch()
	if req.port != 0 {
		assoc.client.Store(&net.UDPAddr{IP: assoc.clientIP, Port: req.port})
	}
	done := make(chan struct{})
	defer close(done)
	wg.Add(1)
	go func() {
		defer wg.Done()
		assoc.closeWhenIdle(ctrl, s.opts.UDPIdleTimeout, done)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.relayFromClient(ctx, pcClient, pcVPN, pcSys, assoc)
	}()
	for _, pc := range conns[1:] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			relayToClient(pc, pcClient, assoc)
		}()
	}
	// RFC 1928: the association ends when the TCP control connection ends.
	_, _ = io.Copy(io.Discard, ctrl)
}

func (s *socksServer) relayFromClient(ctx context.Context, pcClient, pcVPN, pcSys net.PacketConn, assoc *udpAssociation) {
	buf := make([]byte, 65535)
	for {
		n, from, err := pcClient.ReadFrom(buf)
		if err != nil {
			return
		}
		src, ok := from.(*net.UDPAddr)
		if !ok || !src.IP.Equal(assoc.clientIP) {
			continue
		}
		pinned := assoc.client.Load()
		if pinned != nil && pinned.Port != src.Port {
			continue
		}
		if n < 4 || buf[2] != 0 { // fragmented datagrams are not supported
			continue
		}
		r := bytes.NewReader(buf[3:n])
		dst, err := readSocksAddr(r)
		if err != nil {
			continue
		}
		if pinned == nil {
			assoc.client.Store(src)
		}
		payload := buf[n-r.Len() : n]
		var domain string
		if dst.atyp == socksATYPDomain {
			domain = strings.ToLower(dst.host)
		}
		useVPN := s.opts.BindIf != "" && s.routeViaVPN(domain)
		ip := net.ParseIP(dst.host)
		if domain != "" {
			if ip = s.resolve(ctx, dst.host, useVPN); ip == nil {
				continue
			}
		}
		pc := pcSys
		if useVPN {
			pc = pcVPN
		}
		if s.opts.Debug {
			fmt.Printf("[socks-udp] -> %s:%d via %s\n", ip, dst.port, pc.LocalAddr())
		}
		assoc.peers.add(ip, dst.port)
		assoc.touch()
		_, _ = pc.WriteTo(payload, &net.UDPAddr{IP: ip, Port: dst.port})
	}
}

func relayToClient(pc, pcClient net.PacketConn, assoc *udpAssociation) {
	buf := make([]byte, 65535)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		src, ok := from.(*net.UDPAddr)
		dst := assoc.client.Load()
		if !ok || dst == nil || !assoc.peers.has(src) {
			continue
		}
		pkt := appendSocksIP([]byte{0, 0, 0}, src.IP)
		pkt = binary.BigEndian.AppendUint16(pkt, uint16(src.Port))
		pkt = append(pkt, buf[:n]...)
		assoc.touch()
		_, _ = pcClient.WriteTo(pkt, dst)
	}
}

func addrIP(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.TCPAddr:
		return v.IP
	case *net.UDPAddr:
		return v.IP
	}
	return nil
}

var errBindInterface = errors.New("bind to VPN interface failed")

const (
	darwinIPBoundIF     = 25  // IP_BOUND_IF
	darwinIPv6BoundIF   = 125 // IPV6_BOUND_IF
	linuxSOBindToDevice = 25  // SO_BINDTODEVICE, needs CAP_NET_RAW
)

// bindFDToInterface pins a socket to ifName. Any failure is returned so the caller refuses
// the request; silently continuing would send VPN traffic out the normal default route.
func bindFDToInterface(fd int, network, ifName string) error {
	if strings.TrimSpace(ifName) == "" {
		return nil
	}
	ifi, err := net.InterfaceByName(ifName)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errBindInterface, ifName, err)
	}
	switch runtime.GOOS {
	case "darwin":
		level, opt := syscall.IPPROTO_IP, darwinIPBoundIF
		if strings.HasSuffix(network, "6") {
			level, opt = syscall.IPPROTO_IPV6, darwinIPv6BoundIF
		}
		err = syscall.SetsockoptInt(fd, level, opt, ifi.Index)
	case "linux":
		err = syscall.SetsockoptString(fd, syscall.SOL_SOCKET, linuxSOBindToDevice, ifName)
	default:
		err = fmt.Errorf("not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errBindInterface, ifName, err)
	}
	return nil
}
