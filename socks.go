package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
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
}

// StartSocks5 starts a SOCKS5 proxy and returns a stop function.
func StartSocks5(ctx context.Context, opts SocksOptions) (func() error, error) {
	if err := opts.Auth.validate(); err != nil {
		return nil, err
	}
	if opts.HandshakeTimeout <= 0 {
		opts.HandshakeTimeout = defaultSocksHandshakeTimeout
	}
	srv := &socksServer{opts: opts, resolver: newSocksResolver(opts.DNSServers)}
	ln, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		acceptLoop(ctx, ln, time.Second, func(conn net.Conn) { go srv.handleConn(ctx, conn) })
	}()
	stop := func() error { _ = ln.Close(); <-done; return nil }
	return stop, nil
}

type socksServer struct {
	opts     SocksOptions
	resolver *net.Resolver
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
		logWarn("socks accept failed: %v; retrying in %s\n", err, backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

func newSocksResolver(dnsServers []string) *net.Resolver {
	if len(dnsServers) == 0 {
		return net.DefaultResolver
	}
	addr := dnsServers[0]
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "53")
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", addr)
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
		s.runUDPAssociate(ctx, c)
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
		err := socksUserPassAuth(c, s.opts.Auth)
		if errors.Is(err, errSocksAuthFailed) {
			logWarn("socks: rejected credentials from %s\n", c.RemoteAddr())
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
	ip := net.ParseIP(dst.host)
	if dst.atyp == socksATYPDomain {
		domain = strings.ToLower(dst.host)
		if ip = s.resolve(ctx, dst.host); ip == nil {
			_ = writeSocksReply(c, socksRepHostUnreachable, nil)
			return
		}
	}
	target := net.JoinHostPort(ip.String(), strconv.Itoa(dst.port))
	useVPN := s.routeViaVPN(domain)
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
			logWarn("socks: %v; refusing CONNECT to %s instead of sending it outside the VPN\n", err, dst)
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
func (s *socksServer) resolve(ctx context.Context, host string) net.IP {
	addrs, _ := s.resolver.LookupIP(ctx, "ip", host)
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

// runUDPAssociate implements SOCKS5 UDP ASSOCIATE for a single TCP control connection.
func (s *socksServer) runUDPAssociate(ctx context.Context, ctrl net.Conn) {
	bindIf, debug, allowDomains, excludeDomains, resolver := s.opts.BindIf, s.opts.Debug, s.opts.AllowDomains, s.opts.ExcludeDomains, s.resolver
	// Allocate a UDP listener for the client on loopback (IPv4)
	pcClient, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		_ = writeSocksReply(ctrl, 1, nil)
		return
	}
	defer func() { _ = pcClient.Close() }()
	la := pcClient.LocalAddr().(*net.UDPAddr)
	// Reply success with our UDP bind address
	if debug {
		fmt.Printf("[socks] UDP ASSOCIATE listening at %s bindIf=%s\n", la.String(), bindIf)
	}
	// Build BND.ADDR/PORT reply
	resp := []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	copy(resp[4:8], la.IP.To4())
	resp[8] = byte(la.Port >> 8)
	resp[9] = byte(la.Port)
	if _, err := ctrl.Write(resp); err != nil {
		return
	}

	// Prepare outbound UDP packet conns: one bound to VPN interface, one system default
	var pcVPN, pcSys net.PacketConn
	// VPN-bound packet conn
	if bindIf != "" {
		lc := net.ListenConfig{Control: func(network, address string, rc syscall.RawConn) error {
			var retErr error
			ctlErr := rc.Control(func(fd uintptr) {
				retErr = bindFDToInterface(int(fd), network, bindIf)
			})
			if ctlErr != nil {
				return ctlErr
			}
			return retErr
		}}
		pcVPN, _ = lc.ListenPacket(ctx, "udp", ":0")
	}
	// System packet conn
	var errSys error
	pcSys, errSys = net.ListenPacket("udp", ":0")
	if pcVPN == nil && errSys != nil {
		// No viable UDP socket
		return
	}

	// Read from remote sockets and forward to client
	clientAddrCh := make(chan net.Addr, 1)
	// track last client address observed — written from one goroutine, read from another.
	var lastClientAddr atomic.Pointer[net.Addr]
	go func() {
		b := make([]byte, 65535)
		for {
			n, addr, err := pcClient.ReadFrom(b)
			if err != nil {
				return
			}
			lastClientAddr.Store(&addr)
			select {
			case clientAddrCh <- addr:
			default:
			}
			// Parse SOCKS5 UDP request header
			if n < 10 {
				continue
			}
			p := b[:n]
			// RSV(2)=0, FRAG(1)=0, ATYP(1)
			if p[0] != 0 || p[1] != 0 || p[2] != 0 {
				continue
			}
			atyp := p[3]
			off := 4
			var dstIP net.IP
			var dstPort int
			var reqDomain string
			switch atyp {
			case 1: // IPv4
				if len(p) < off+4+2 {
					continue
				}
				dstIP = net.IP(p[off : off+4])
				off += 4
			case 3: // Domain
				if len(p) < off+1 {
					continue
				}
				l := int(p[off])
				off++
				if len(p) < off+l+2 {
					continue
				}
				reqDomain = strings.ToLower(string(p[off : off+l]))
				off += l
			case 4: // IPv6
				if len(p) < off+16+2 {
					continue
				}
				dstIP = net.IP(p[off : off+16])
				off += 16
			default:
				continue
			}
			dstPort = int(p[off])<<8 | int(p[off+1])
			off += 2
			payload := p[off:]

			// Resolve domain if needed
			if dstIP == nil && reqDomain != "" {
				addrs, _ := resolver.LookupIP(ctx, "ip", reqDomain)
				for _, ip := range addrs {
					if ip.To4() != nil {
						dstIP = ip
						break
					}
					if dstIP == nil {
						dstIP = ip
					}
				}
				if dstIP == nil {
					continue
				}
			}

			// Decide path
			useVPN := true
			if len(allowDomains) > 0 {
				if reqDomain == "" || !domainMatches(reqDomain, allowDomains) {
					useVPN = false
				}
			}
			if reqDomain != "" && domainMatches(reqDomain, excludeDomains) {
				useVPN = false
			}
			if debug {
				fmt.Printf("[socks-udp] -> %s:%d via %s\n", dstIP, dstPort, map[bool]string{true: bindIf, false: "system"}[useVPN])
			}

			// Send out
			dst := &net.UDPAddr{IP: dstIP, Port: dstPort}
			var pc net.PacketConn
			if useVPN && pcVPN != nil {
				pc = pcVPN
			} else {
				pc = pcSys
			}
			if pc == nil {
				continue
			}
			_, _ = pc.WriteTo(payload, dst)
		}
	}()

	// Forward replies from VPN/system sockets back to client with SOCKS header
	sendBack := func(pc net.PacketConn) {
		if pc == nil {
			return
		}
		buf := make([]byte, 65535)
		for {
			n, raddr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			// Build SOCKS UDP response header
			addrPtr := lastClientAddr.Load()
			if addrPtr == nil {
				// Wait for at least one client packet to learn client addr
				select {
				case a := <-clientAddrCh:
					lastClientAddr.Store(&a)
					addrPtr = &a
				default:
				}
				if addrPtr == nil {
					continue
				}
			}
			hdr := make([]byte, 0, 10)
			hdr = append(hdr, 0, 0, 0) // RSV, RSV, FRAG
			// raddr can be UDPAddr
			if ua, ok := raddr.(*net.UDPAddr); ok {
				ip := ua.IP
				if v4 := ip.To4(); v4 != nil {
					hdr = append(hdr, 1)
					hdr = append(hdr, v4...)
				} else {
					hdr = append(hdr, 4)
					hdr = append(hdr, ip.To16()...)
				}
				hdr = append(hdr, byte(ua.Port>>8), byte(ua.Port))
			} else {
				continue
			}
			pkt := append(hdr, buf[:n]...)
			_, _ = pcClient.WriteTo(pkt, *addrPtr)
		}
	}
	go sendBack(pcVPN)
	go sendBack(pcSys)

	// Keep TCP control channel open until client closes
	tmp := make([]byte, 1)
	_, _ = ctrl.Read(tmp)
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
