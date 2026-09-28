package tunnel

import (
	"net"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// shouldDropInbound returns true when the packet should be dropped by inbound filtering.
// allowCIDRs is the list of permitted source CIDRs for new inbound connections.
// If allowCIDRs is empty, all new inbound TCP SYNs are dropped.
// Both IPv4 and IPv6 TCP packets are inspected; all other protocols pass through.
func shouldDropInbound(packet []byte, allowCIDRs []*net.IPNet) bool {
	if len(packet) < 1 {
		return false
	}
	version := packet[0] >> 4
	switch version {
	case 4:
		return shouldDropInboundIPv4(packet, allowCIDRs)
	case 6:
		return shouldDropInboundIPv6(packet, allowCIDRs)
	default:
		return false
	}
}

func shouldDropInboundIPv4(packet []byte, allowCIDRs []*net.IPNet) bool {
	if len(packet) < 20 {
		return false
	}
	ihl := int(packet[0]&0x0F) * 4
	if ihl < 20 || len(packet) < ihl+20 {
		return false
	}
	if packet[9] != 6 { // not TCP
		return false
	}
	tcpFlags := packet[ihl+13]
	syn := tcpFlags&0x02 != 0
	ack := tcpFlags&0x10 != 0
	if syn && !ack {
		// New inbound SYN: drop unless source IP is in the allowlist.
		if len(allowCIDRs) > 0 {
			srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
			for _, n := range allowCIDRs {
				if n != nil && n.Contains(srcIP) {
					return false // allowed
				}
			}
		}
		return true // drop
	}
	if !ack {
		// Non-SYN TCP without ACK (e.g., stray RST): drop.
		return true
	}
	return false
}

// shouldDropInboundIPv6 applies the same SYN/ACK policy to IPv6 TCP packets.
// IPv6 fixed header is 40 bytes; Next Header field is at byte 6.
// Extension headers are not walked — packets with non-TCP Next Header pass through.
func shouldDropInboundIPv6(packet []byte, allowCIDRs []*net.IPNet) bool {
	const ipv6HeaderLen = 40
	if len(packet) < ipv6HeaderLen+20 {
		return false
	}
	if packet[6] != 6 { // Next Header must be TCP (6)
		return false
	}
	tcpOffset := ipv6HeaderLen
	tcpFlags := packet[tcpOffset+13]
	syn := tcpFlags&0x02 != 0
	ack := tcpFlags&0x10 != 0
	if syn && !ack {
		if len(allowCIDRs) > 0 {
			srcIP := net.IP(packet[8:24]) // bytes 8–23 are the source address
			for _, n := range allowCIDRs {
				if n != nil && n.Contains(srcIP) {
					return false
				}
			}
		}
		return true
	}
	if !ack {
		return true
	}
	return false
}

// parseCIDRHost parses a CIDR or single host address (IPv4 or IPv6) into *net.IPNet.
// Returns nil on failure.
func parseCIDRHost(s string) *net.IPNet {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil
		}
		if ip4 := ip.To4(); ip4 != nil {
			return &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}
		}
		return &net.IPNet{IP: ip.To16(), Mask: net.CIDRMask(128, 128)}
	}
	_, ipn, err := net.ParseCIDR(s)
	if err != nil {
		return nil
	}
	return ipn
}

// InboundPolicy decides which provider-to-TUN TCP packets are dropped before they reach the host.
type InboundPolicy struct {
	enabled bool
	allow   []*net.IPNet
}

// NewInboundPolicy builds the allowlist from --allow_inbound_src and --allow_inbound_local.
func NewInboundPolicy(allowSrcList string, allowLocal bool, tunCIDR string) InboundPolicy {
	p := InboundPolicy{enabled: allowLocal || allowSrcList != ""}
	add := func(cidr string) {
		if n := parseCIDRHost(cidr); n != nil {
			p.allow = append(p.allow, n)
		}
	}
	if allowSrcList != "" {
		for _, s := range strings.Split(allowSrcList, ",") {
			add(s)
		}
	}
	if allowLocal {
		for _, cidr := range []string{"127.0.0.0/8", "169.254.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"} {
			add(cidr)
		}
		if tunCIDR != "" {
			add(tunCIDR)
		}
	}
	if p.enabled && logx.IsInfoEnabled() {
		logx.Info("inbound-control: enabled (allowlist=%d entries); policy: drop new inbound SYN not in allowlist, and drop inbound TCP without ACK\n", len(p.allow))
	}
	return p
}
