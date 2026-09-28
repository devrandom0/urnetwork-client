package tunnel

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// Counters are updated atomically by the dataplane and read by stats and the DNS bootstrap.
type Counters struct {
	PktsIn, PktsOut, BytesIn, BytesOut uint64
}

// Dataplane moves packets between the TUN device and the provider connection.
type Dataplane struct {
	dev        io.ReadWriter
	policy     InboundPolicy
	enableIPv6 bool
	counters   *Counters
}

func NewDataplane(dev io.ReadWriter, policy InboundPolicy, enableIPv6 bool, c *Counters) *Dataplane {
	return &Dataplane{dev: dev, policy: policy, enableIPv6: enableIPv6, counters: c}
}

// Receive filters one packet from the provider and writes it to the TUN.
func (d *Dataplane) Receive(packet []byte) {
	if len(packet) > 0 && packet[0]>>4 == 6 && !d.enableIPv6 {
		if logx.IsDebugEnabled() {
			logx.Debug("dropped IPv6 packet (--enable_ipv6 not set)\n")
		}
		return
	}
	if d.policy.enabled && shouldDropInbound(packet, d.policy.allow) {
		if logx.IsDebugEnabled() {
			logDroppedInbound(packet)
		}
		return
	}
	_, _ = d.dev.Write(packet)
	atomic.AddUint64(&d.counters.PktsIn, 1)
	atomic.AddUint64(&d.counters.BytesIn, uint64(len(packet)))
}

func logDroppedInbound(packet []byte) {
	version := byte(0)
	if len(packet) > 0 {
		version = packet[0] >> 4
	}
	if version == 4 && len(packet) >= 20 {
		ihl := int(packet[0]&0x0F) * 4
		if ihl >= 20 && len(packet) >= ihl+20 && packet[9] == 6 {
			srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
			dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19])
			srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
			dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
			tcpFlags := packet[ihl+13]
			logx.Debug("dropped inbound TCP %s:%d -> %s:%d (flags=0x%02x)\n", srcIP, srcPort, dstIP, dstPort, tcpFlags)
		}
	} else if version == 6 && len(packet) >= 60 && packet[6] == 6 {
		srcIP := net.IP(packet[8:24])
		dstIP := net.IP(packet[24:40])
		srcPort := binary.BigEndian.Uint16(packet[40:42])
		dstPort := binary.BigEndian.Uint16(packet[42:44])
		tcpFlags := packet[53]
		logx.Debug("dropped inbound TCP6 %s:%d -> %s:%d (flags=0x%02x)\n", srcIP, srcPort, dstIP, dstPort, tcpFlags)
	}
}

// PumpOutbound copies packets from the TUN to send until a device read fails.
func (d *Dataplane) PumpOutbound(send func(packet []byte)) {
	buf := make([]byte, 65536)
	for {
		n, err := d.dev.Read(buf)
		if err != nil {
			return
		}
		if n <= 0 {
			continue
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		logx.Debug("-> provider len=%d\n", len(pkt))
		send(pkt)
		atomic.AddUint64(&d.counters.PktsOut, 1)
		atomic.AddUint64(&d.counters.BytesOut, uint64(len(pkt)))
	}
}

// LogStats prints the counters every interval until ctx ends.
func LogStats(ctx context.Context, interval time.Duration, c *Counters) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			inP := atomic.LoadUint64(&c.PktsIn)
			inB := atomic.LoadUint64(&c.BytesIn)
			outP := atomic.LoadUint64(&c.PktsOut)
			outB := atomic.LoadUint64(&c.BytesOut)
			logx.Info("[stats] in=%d pkts / %d bytes, out=%d pkts / %d bytes\n", inP, inB, outP, outB)
		}
	}
}
