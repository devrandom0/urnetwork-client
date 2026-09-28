package tunnel

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeDev struct {
	mu     sync.Mutex
	writes [][]byte
	reads  chan []byte
}

func (d *fakeDev) Read(p []byte) (int, error) {
	pkt, ok := <-d.reads
	if !ok {
		return 0, io.EOF
	}
	return copy(p, pkt), nil
}

func (d *fakeDev) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.writes = append(d.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (d *fakeDev) written() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.writes)
}

func TestDataplane_ReceiveDropsIPv6WhenDisabled(t *testing.T) {
	dev := &fakeDev{}
	var c Counters
	dp := NewDataplane(dev, NewInboundPolicy("", false, ""), false, &c)
	dp.Receive(buildIPv6TCPPacket(net.ParseIP("2001:db8::2"), 0x10))
	if dev.written() != 0 || atomic.LoadUint64(&c.PktsIn) != 0 {
		t.Fatalf("IPv6 packet reached the TUN with --enable_ipv6 off (writes=%d)", dev.written())
	}
}

func TestDataplane_ReceiveAppliesInboundPolicy(t *testing.T) {
	dev := &fakeDev{}
	var c Counters
	dp := NewDataplane(dev, NewInboundPolicy("10.0.0.0/8", false, ""), false, &c)
	dp.Receive(buildTCPPacket(net.ParseIP("8.8.8.8"), 0x02))
	allowed := buildTCPPacket(net.ParseIP("10.1.2.3"), 0x02)
	dp.Receive(allowed)
	if dev.written() != 1 {
		t.Fatalf("writes = %d, want only the allowlisted SYN", dev.written())
	}
	if atomic.LoadUint64(&c.PktsIn) != 1 || atomic.LoadUint64(&c.BytesIn) != uint64(len(allowed)) {
		t.Fatalf("counters = %+v", &c)
	}
}

func TestNewInboundPolicy_DisabledPassesInboundSYN(t *testing.T) {
	dev := &fakeDev{}
	var c Counters
	dp := NewDataplane(dev, NewInboundPolicy("", false, ""), false, &c)
	dp.Receive(buildTCPPacket(net.ParseIP("8.8.8.8"), 0x02))
	if dev.written() != 1 {
		t.Fatal("inbound control is off without --allow_inbound_*; the SYN must pass")
	}
}

func TestDataplane_PumpOutboundForwardsUntilReadFails(t *testing.T) {
	dev := &fakeDev{reads: make(chan []byte, 2)}
	dev.reads <- []byte{0x45, 1, 2}
	dev.reads <- []byte{0x45, 1, 2, 3}
	close(dev.reads)
	var c Counters
	var sent [][]byte
	NewDataplane(dev, NewInboundPolicy("", false, ""), false, &c).PumpOutbound(func(p []byte) { sent = append(sent, p) })
	if len(sent) != 2 || atomic.LoadUint64(&c.PktsOut) != 2 || atomic.LoadUint64(&c.BytesOut) != 7 {
		t.Fatalf("sent=%d counters=%+v", len(sent), &c)
	}
}
