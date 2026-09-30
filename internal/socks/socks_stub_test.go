package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeDNS answers A queries with ip and every other type with an empty NOERROR, and
// records the query types it saw.
type fakeDNS struct {
	pc     net.PacketConn
	mu     sync.Mutex
	qtypes []uint16
}

func startFakeDNS(t *testing.T, ip net.IP) *fakeDNS {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeDNS{pc: pc}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if resp, qtype, ok := fakeDNSReply(buf[:n], ip.To4()); ok {
				f.mu.Lock()
				f.qtypes = append(f.qtypes, qtype)
				f.mu.Unlock()
				_, _ = pc.WriteTo(resp, from)
			}
		}
	}()
	return f
}

func (f *fakeDNS) addr() string { return f.pc.LocalAddr().String() }

func (f *fakeDNS) seen() []uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint16(nil), f.qtypes...)
}

func fakeDNSReply(q []byte, ip net.IP) ([]byte, uint16, bool) {
	i := 12
	for i < len(q) && q[i] != 0 {
		i += int(q[i]) + 1
	}
	if i+5 > len(q) {
		return nil, 0, false
	}
	qtype := binary.BigEndian.Uint16(q[i+1 : i+3])
	qend := i + 5
	resp := append([]byte{q[0], q[1], 0x81, 0x80, 0, 1, 0, 0, 0, 0, 0, 0}, q[12:qend]...)
	if qtype == 1 {
		resp[7] = 1
		resp = append(resp, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
		resp = append(resp, ip...)
	}
	return resp, qtype, true
}

func TestSocksResolve_StubIsUsedWithoutInterfaceBinding(t *testing.T) {
	stub := startFakeDNS(t, net.ParseIP("192.0.2.77"))
	s := &socksServer{opts: SocksOptions{BindIf: "urnet-nope0", DNSStub: stub.addr(), DNSServers: []string{"192.0.2.53"}}}
	s.initResolvers()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for _, viaVPN := range []bool{true, false} {
		if ip := s.resolve(ctx, "stub.example.test", viaVPN); ip == nil || ip.String() != "192.0.2.77" {
			t.Fatalf("resolve(viaVPN=%v) = %v; want the stub's answer", viaVPN, ip)
		}
	}
}

func TestSocksResolve_IPv4OnlyAsksForARecordsOnly(t *testing.T) {
	stub := startFakeDNS(t, net.ParseIP("192.0.2.78"))
	s := &socksServer{opts: SocksOptions{DNSStub: stub.addr(), IPv4Only: true}}
	s.initResolvers()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if ip := s.resolve(ctx, "v4.example.test", true); ip == nil {
		t.Fatal("resolve returned nothing")
	}
	for _, qt := range stub.seen() {
		if qt != 1 {
			t.Fatalf("query types = %v; with IPv6 off only A (1) may be asked", stub.seen())
		}
	}
}

func TestInterfaceControl_FailsDialForMissingInterface(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	d := net.Dialer{Timeout: time.Second, Control: InterfaceControl("urnet-nope0")}

	c, err := d.Dial("tcp4", ln.Addr().String())
	if err == nil {
		_ = c.Close()
		t.Fatal("dial succeeded although the socket could not be pinned to the interface")
	}
	if !errors.Is(err, errBindInterface) {
		t.Fatalf("err = %v, want errBindInterface", err)
	}
}
