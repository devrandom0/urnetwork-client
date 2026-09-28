package socks

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func startSocksForTest(t *testing.T, opts SocksOptions) string {
	t.Helper()
	if opts.ListenAddr == "" {
		opts.ListenAddr = grabFreeAddr(t)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stop, err := StartSocks5(ctx, opts)
	if err != nil {
		cancel()
		t.Fatalf("StartSocks5: %v", err)
	}
	t.Cleanup(func() {
		_ = stop()
		cancel()
	})
	return opts.ListenAddr
}

func startTCPEcho(t *testing.T, network, addr string) *net.TCPAddr {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Skipf("cannot listen on %s %s: %v", network, addr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr)
}

func dialSocks(t *testing.T, proxy string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxy, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func socksGreet(t *testing.T, c net.Conn, methods ...byte) byte {
	t.Helper()
	if _, err := c.Write(append([]byte{5, byte(len(methods))}, methods...)); err != nil {
		t.Fatalf("write greeting: %v", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("read method selection: %v", err)
	}
	if resp[0] != 5 {
		t.Fatalf("bad version in method selection: %v", resp)
	}
	return resp[1]
}

func encodeSocksAddr(host string, port int) []byte {
	var b []byte
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			b = append([]byte{1}, v4...)
		} else {
			b = append([]byte{4}, ip.To16()...)
		}
	} else {
		b = append([]byte{3, byte(len(host))}, host...)
	}
	return append(b, byte(port>>8), byte(port))
}

func socksRequest(t *testing.T, c net.Conn, cmd byte, host string, port int) (byte, socksAddr) {
	t.Helper()
	req := append([]byte{5, cmd, 0}, encodeSocksAddr(host, port)...)
	if _, err := c.Write(req); err != nil {
		t.Fatalf("write request: %v", err)
	}
	hdr := make([]byte, 3)
	if _, err := io.ReadFull(c, hdr); err != nil {
		t.Fatalf("read reply header: %v", err)
	}
	bnd, err := readSocksAddr(c)
	if err != nil {
		t.Fatalf("read reply address: %v", err)
	}
	return hdr[1], bnd
}

func assertSocksEcho(t *testing.T, proxy, host string, port int) {
	t.Helper()
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, socksMethodNoAuth); m != socksMethodNoAuth {
		t.Fatalf("method = %#x", m)
	}
	rep, bnd := socksRequest(t, c, socksCmdConnect, host, port)
	if rep != socksRepSucceeded {
		t.Fatalf("CONNECT %s:%d rep = %d", host, port, rep)
	}
	if bnd.port == 0 {
		t.Fatal("reply BND.PORT is 0; bind address was not encoded")
	}
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatalf("write through tunnel: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}
