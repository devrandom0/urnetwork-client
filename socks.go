package main

import (
    "context"
    "errors"
    "fmt"
    "io"
    "net"
    "strconv"
    "strings"
    "syscall"
    "time"
)

// StartSocks5 starts a minimal SOCKS5 proxy at listenAddr.
// If bindIf is non-empty (e.g., "utun10" or "tun0"), outbound connections will be attempted with SO_BINDTODEVICE where supported
// (Linux) or using a control on macOS to set IP_BOUND_IF via syscall.RawConn.Control.
func StartSocks5(
    ctx context.Context,
    listenAddr string,
    bindIf string,
    debug bool,
    allowDomains []string,
    excludeDomains []string,
) (func() error, error) {
    ln, err := net.Listen("tcp", listenAddr)
    if err != nil { return nil, err }
    done := make(chan struct{})
    go func() {
        defer close(done)
        for {
            conn, err := ln.Accept()
            if err != nil {
                if ne, ok := err.(net.Error); ok && ne.Temporary() { continue }
                return
            }
            go handleSocksConn(ctx, conn, bindIf, debug, allowDomains, excludeDomains)
        }
    }()
    stop := func() error { _ = ln.Close(); <-done; return nil }
    return stop, nil
}

func handleSocksConn(
    ctx context.Context,
    c net.Conn,
    bindIf string,
    debug bool,
    allowDomains []string,
    excludeDomains []string,
) {
    defer c.Close()

    // RFC 1928 greeting
    // +----+----------+----------+
    // |VER | NMETHODS | METHODS  |
    // +----+----------+----------+
    buf := make([]byte, 262)
    if _, err := io.ReadFull(c, buf[:2]); err != nil { return }
    ver, nMethods := buf[0], int(buf[1])
    if ver != 5 { return }
    if _, err := io.ReadFull(c, buf[:nMethods]); err != nil { return }
    // no auth
    if _, err := c.Write([]byte{5, 0}); err != nil { return }

    // Request
    // +----+-----+-------+------+----------+----------+
    // |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
    // +----+-----+-------+------+----------+----------+
    if _, err := io.ReadFull(c, buf[:4]); err != nil { return }
    ver, cmd, atyp := buf[0], buf[1], buf[3]
    if ver != 5 { _ = writeSocksReply(c, 1, nil); return }
    if cmd == 3 { // UDP ASSOCIATE
        runUDPAssociate(ctx, c, bindIf, debug, allowDomains, excludeDomains)
        return
    }
    if cmd != 1 { // CONNECT only
        _ = writeSocksReply(c, 7, nil)
        return
    }
    var host string
    switch atyp {
    case 1: // IPv4
        if _, err := io.ReadFull(c, buf[:4]); err != nil { return }
        host = net.IP(buf[:4]).String()
    case 3: // domain
        if _, err := io.ReadFull(c, buf[:1]); err != nil { return }
        l := int(buf[0])
        if _, err := io.ReadFull(c, buf[:l]); err != nil { return }
        host = string(buf[:l])
    case 4: // IPv6
        if _, err := io.ReadFull(c, buf[:16]); err != nil { return }
        host = net.IP(buf[:16]).String()
    default:
        _ = writeSocksReply(c, 8, nil)
        return
    }
    if _, err := io.ReadFull(c, buf[:2]); err != nil { return }
    port := int(buf[0])<<8 | int(buf[1])
    // Resolve target address to an IP for routing. Prefer IPv4
    var ipForRoute net.IP
    var addr string
    var reqDomain string
    if atyp == 3 { // domain
        // Use system resolver
        reqDomain = strings.ToLower(host)
        addrs, _ := net.DefaultResolver.LookupIP(ctx, "ip", host)
        for _, ip := range addrs {
            if ip.To4() != nil { ipForRoute = ip; break }
            if ipForRoute == nil { ipForRoute = ip }
        }
        if ipForRoute == nil {
            _ = writeSocksReply(c, 4, nil) // host unreachable
            return
        }
        addr = net.JoinHostPort(ipForRoute.String(), strconv.Itoa(port))
    } else {
        ipForRoute = net.ParseIP(host)
        addr = net.JoinHostPort(host, strconv.Itoa(port))
    }
    useVPN := true
    if len(allowDomains) > 0 {
        if reqDomain == "" || !domainMatches(reqDomain, allowDomains) { useVPN = false }
    }
    if reqDomain != "" && domainMatches(reqDomain, excludeDomains) { useVPN = false }
    if debug { fmt.Printf("[socks] CONNECT %s (ip=%s) bindIf=%s useVPN=%v\n", host+":"+strconv.Itoa(port), ipForRoute, bindIf, useVPN) }

    d := net.Dialer{ Timeout: 30 * time.Second }
    if bindIf != "" {
        // Platform-specific binding
        d.Control = func(network, address string, c syscall.RawConn) error {
            var cerr error
            if strings.Contains(network, "tcp") {
                err := c.Control(func(fd uintptr) {
                    // macOS: IP_BOUND_IF (if_nametoindex)
                    ifi, _ := net.InterfaceByName(bindIf)
                    if ifi != nil {
                        // IPPROTO_IP=0, IP_BOUND_IF=25
                        cerr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, 25, ifi.Index)
                        return
                    }
                    // Linux: SO_BINDTODEVICE
                    // 15 is SO_BINDTODEVICE; requires root
                    _ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, 25, bindIf)
                })
                if err != nil { return err }
            }
            return cerr
        }
    }
    // Add a per-destination host route via the VPN interface if provided
    if useVPN && bindIf != "" {
        d.Control = func(network, address string, c syscall.RawConn) error {
            var cerr error
            if strings.Contains(network, "tcp") {
                err := c.Control(func(fd uintptr) {
                    ifi, _ := net.InterfaceByName(bindIf)
                    if ifi != nil {
                        cerr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, 25, ifi.Index)
                        return
                    }
                    _ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, 25, bindIf)
                })
                if err != nil { return err }
            }
            return cerr
        }
    }

    rc, err := d.DialContext(ctx, "tcp", addr)
    if err != nil {
        // Map common errors to SOCKS reply codes
        rep := byte(1) // general failure by default
        if ne, ok := err.(net.Error); ok && ne.Timeout() { rep = 4 }
        var se syscall.Errno
        if errors.As(err, &se) {
            switch se {
            case syscall.ECONNREFUSED: rep = 5
            case syscall.ENETUNREACH: rep = 3
            case syscall.EHOSTUNREACH: rep = 4
            case syscall.ETIMEDOUT: rep = 4
            }
        }
        if debug { fmt.Printf("[socks] dial error to %s: %v (rep=%d)\n", addr, err, rep) }
        _ = writeSocksReply(c, rep, nil)
        return
    }
    defer rc.Close()
    if err := writeSocksReply(c, 0, rc.LocalAddr()); err != nil { return }
    // pipe
    go io.Copy(rc, c)
    io.Copy(c, rc)
}

func writeSocksReply(c net.Conn, rep byte, bindAddr net.Addr) error {
    // Minimal reply with 0.0.0.0:0
    resp := []byte{5, rep, 0, 1, 0, 0, 0, 0, 0, 0}
    _, err := c.Write(resp)
    return err
}

// domainMatches checks if host matches any suffix in patterns (case-insensitive).
func domainMatches(host string, patterns []string) bool {
    h := strings.ToLower(strings.TrimSuffix(host, "."))
    for _, p := range patterns {
        p = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(p, ".")))
        if p == "" { continue }
        if h == p || strings.HasSuffix(h, "."+p) { return true }
    }
    return false
}

// runUDPAssociate implements SOCKS5 UDP ASSOCIATE for a single TCP control connection.
func runUDPAssociate(ctx context.Context, ctrl net.Conn, bindIf string, debug bool, allowDomains, excludeDomains []string) {
    // Allocate a UDP listener for the client on loopback (IPv4)
    pcClient, err := net.ListenPacket("udp", "127.0.0.1:0")
    if err != nil { _ = writeSocksReply(ctrl, 1, nil); return }
    defer pcClient.Close()
    la := pcClient.LocalAddr().(*net.UDPAddr)
    // Reply success with our UDP bind address
    if debug { fmt.Printf("[socks] UDP ASSOCIATE listening at %s bindIf=%s\n", la.String(), bindIf) }
    // Build BND.ADDR/PORT reply
    resp := []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}
    copy(resp[4:8], la.IP.To4())
    resp[8] = byte(la.Port >> 8)
    resp[9] = byte(la.Port)
    if _, err := ctrl.Write(resp); err != nil { return }

    // Prepare outbound UDP packet conns: one bound to VPN interface, one system default
    var pcVPN, pcSys net.PacketConn
    // VPN-bound packet conn
    if bindIf != "" {
        lc := net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
            var cerr error
            err := c.Control(func(fd uintptr) {
                ifi, _ := net.InterfaceByName(bindIf)
                if ifi != nil {
                    // IPv4 binding
                    _ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, 25, ifi.Index) // IP_BOUND_IF
                    // Best-effort IPv6 binding (125)
                    // _ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, 125, ifi.Index)
                } else {
                    // Linux SO_BINDTODEVICE (25)
                    _ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, 25, bindIf)
                }
            })
            if err != nil { return err }
            return cerr
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
    // track last client address observed
    var lastClientAddr net.Addr
    go func() {
        b := make([]byte, 65535)
        for {
            n, addr, err := pcClient.ReadFrom(b)
            if err != nil { return }
            lastClientAddr = addr
            select { case clientAddrCh <- addr: default: }
            // Parse SOCKS5 UDP request header
            if n < 10 { continue }
            p := b[:n]
            // RSV(2)=0, FRAG(1)=0, ATYP(1)
            if p[0] != 0 || p[1] != 0 || p[2] != 0 { continue }
            atyp := p[3]
            off := 4
            var dstIP net.IP
            var dstPort int
            var reqDomain string
            switch atyp {
            case 1: // IPv4
                if len(p) < off+4+2 { continue }
                dstIP = net.IP(p[off : off+4])
                off += 4
            case 3: // Domain
                if len(p) < off+1 { continue }
                l := int(p[off])
                off++
                if len(p) < off+l+2 { continue }
                reqDomain = strings.ToLower(string(p[off : off+l]))
                off += l
            case 4: // IPv6
                if len(p) < off+16+2 { continue }
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
                addrs, _ := net.DefaultResolver.LookupIP(ctx, "ip", reqDomain)
                for _, ip := range addrs { if ip.To4() != nil { dstIP = ip; break }; if dstIP == nil { dstIP = ip } }
                if dstIP == nil { continue }
            }

            // Decide path
            useVPN := true
            if len(allowDomains) > 0 {
                if reqDomain == "" || !domainMatches(reqDomain, allowDomains) { useVPN = false }
            }
            if reqDomain != "" && domainMatches(reqDomain, excludeDomains) { useVPN = false }
            if debug { fmt.Printf("[socks-udp] -> %s:%d via %s\n", dstIP, dstPort, map[bool]string{true: bindIf, false: "system"}[useVPN]) }

            // Send out
            dst := &net.UDPAddr{IP: dstIP, Port: dstPort}
            var pc net.PacketConn
            if useVPN && pcVPN != nil { pc = pcVPN } else { pc = pcSys }
            if pc == nil { continue }
            _, _ = pc.WriteTo(payload, dst)
        }
    }()

    // Forward replies from VPN/system sockets back to client with SOCKS header
    sendBack := func(pc net.PacketConn) {
        if pc == nil { return }
        buf := make([]byte, 65535)
        for {
            n, raddr, err := pc.ReadFrom(buf)
            if err != nil { return }
            // Build SOCKS UDP response header
            if lastClientAddr == nil {
                // Wait for at least one client packet to learn client addr
                select { case lastClientAddr = <-clientAddrCh: default: }
                if lastClientAddr == nil { continue }
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
            _, _ = pcClient.WriteTo(pkt, lastClientAddr)
        }
    }
    go sendBack(pcVPN)
    go sendBack(pcSys)

    // Keep TCP control channel open until client closes
    tmp := make([]byte, 1)
    _, _ = ctrl.Read(tmp)
}
