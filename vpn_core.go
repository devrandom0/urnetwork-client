package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/songgao/water"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/connect/protocol"
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

// vpnRunCore encapsulates the common dataplane loop, provider selection, stats, and SOCKS handling.
// It waits for termination and then invokes onBeforeExit for OS-specific cleanup.
func vpnRunCore(
	ctx context.Context,
	dev *water.Interface,
	tunIfName string,
	cfg VPNConfig,
	pktsIn, pktsOut, bytesIn, bytesOut *uint64,
	onBeforeExit func(),
) {
	apiURL := cfg.APIURL
	connectURL := cfg.ConnectURL
	debugOn := cfg.Debug
	statsInt := cfg.StatsInterval

	// Build provider specs from location flags.
	strat, specs := buildProviderSpecs(ctx, apiURL, cfg.JWT, cfg.Location)
	appVer := fmt.Sprintf("urnet-client %s", Version)
	gen := connect.NewApiMultiClientGeneratorWithDefaults(
		ctx, specs, strat, nil, apiURL, cfg.JWT, fmt.Sprintf("%s/", connectURL), "", "", appVer, nil,
	)

	// Inbound connection control: build allowlist from config.
	blockNewInbound := cfg.AllowInboundLocal || cfg.AllowInboundSrcList != ""
	var allowInboundCIDRs []*net.IPNet
	if cfg.AllowInboundSrcList != "" {
		for _, s := range splitCSV(cfg.AllowInboundSrcList) {
			if n := parseCIDRHost(s); n != nil {
				allowInboundCIDRs = append(allowInboundCIDRs, n)
			}
		}
	}
	if cfg.AllowInboundLocal {
		appendNet := func(cidr string) {
			if n := parseCIDRHost(cidr); n != nil {
				allowInboundCIDRs = append(allowInboundCIDRs, n)
			}
		}
		appendNet("127.0.0.0/8")
		appendNet("169.254.0.0/16")
		appendNet("10.0.0.0/8")
		appendNet("172.16.0.0/12")
		appendNet("192.168.0.0/16")
		appendNet("100.64.0.0/10")
		if cidr := cfg.IPCIDR; cidr != "" {
			if n := parseCIDRHost(cidr); n != nil {
				allowInboundCIDRs = append(allowInboundCIDRs, n)
			}
		}
	}

	if blockNewInbound && isInfoEnabled() {
		logInfo("inbound-control: enabled (allowlist=%d entries); policy: drop new inbound SYN not in allowlist, and drop inbound TCP without ACK\n", len(allowInboundCIDRs))
	}

	// Provider receive: optional userspace filtering, then write to TUN and update counters.
	receive := func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		logDebug("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		
		// Drop IPv6 packets if IPv6 is not enabled.
		if len(packet) > 0 {
			version := packet[0] >> 4
			if version == 6 && !cfg.EnableIPv6 {
				if isDebugEnabled() {
					logDebug("dropped IPv6 packet (--enable_ipv6 not set)\n")
				}
				return
			}
		}
		
		if blockNewInbound && shouldDropInbound(packet, allowInboundCIDRs) {
			if isDebugEnabled() {
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
						logDebug("dropped inbound TCP %s:%d -> %s:%d (flags=0x%02x)\n", srcIP, srcPort, dstIP, dstPort, tcpFlags)
					}
				} else if version == 6 && len(packet) >= 60 && packet[6] == 6 {
					srcIP := net.IP(packet[8:24])
					dstIP := net.IP(packet[24:40])
					srcPort := binary.BigEndian.Uint16(packet[40:42])
					dstPort := binary.BigEndian.Uint16(packet[42:44])
					tcpFlags := packet[53]
					logDebug("dropped inbound TCP6 %s:%d -> %s:%d (flags=0x%02x)\n", srcIP, srcPort, dstIP, dstPort, tcpFlags)
				}
			}
			return
		}
		_, _ = dev.Write(packet)
		if pktsIn != nil {
			atomic.AddUint64(pktsIn, 1)
		}
		if bytesIn != nil {
			atomic.AddUint64(bytesIn, uint64(len(packet)))
		}
	}

	mc := connect.NewRemoteUserNatMultiClientWithDefaults(ctx, gen, receive, protocol.ProvideMode_Network)
	_ = mc

	// TUN -> provider loop
	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := dev.Read(buf)
			if err != nil {
				return
			}
			if n <= 0 {
				continue
			}
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			logDebug("-> provider len=%d\n", len(pkt))
			// No egress userspace filtering
			mc.SendPacket(connect.TransferPath{}, protocol.ProvideMode_Network, pkt, -1)
			if pktsOut != nil {
				atomic.AddUint64(pktsOut, 1)
			}
			if bytesOut != nil {
				atomic.AddUint64(bytesOut, uint64(len(pkt)))
			}
		}
	}()

	// Periodic stats
	if statsInt > 0 && isInfoEnabled() && pktsIn != nil && bytesIn != nil && pktsOut != nil && bytesOut != nil {
		go func() {
			t := time.NewTicker(statsInt)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					inP := atomic.LoadUint64(pktsIn)
					inB := atomic.LoadUint64(bytesIn)
					outP := atomic.LoadUint64(pktsOut)
					outB := atomic.LoadUint64(bytesOut)
					logInfo("[stats] in=%d pkts / %d bytes, out=%d pkts / %d bytes\n", inP, inB, outP, outB)
				}
			}
		}()
	}

	// Optional SOCKS5 proxy bound to the VPN interface
	socksListen := cfg.SOCKSListen
	allowDomains := cfg.AllowDomains
	excludeDomains := cfg.ExcludeDomains
	var stopSocks func() error
	if socksListen != "" {
		if s, err := StartSocks5(ctx, socksListen, tunIfName, debugOn || isDebugEnabled(), allowDomains, excludeDomains); err != nil {
			logWarn("failed to start socks at %s: %v\n", socksListen, err)
		} else {
			stopSocks = s
			logInfo("SOCKS5 listening at %s (bound to %s)\n", socksListen, tunIfName)
		}
	}

	if isInfoEnabled() {
		fmt.Println("VPN dataplane running; press Ctrl-C to exit.")
	}

	// Wait for termination via context cancellation
	<-ctx.Done()

	// Cleanup order: stop socks, then OS-specific cleanup
	if stopSocks != nil {
		_ = stopSocks()
	}
	if onBeforeExit != nil {
		onBeforeExit()
	}
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

// isTUNDisabled returns true when the provided name represents a disabled TUN
// (e.g., "none", "no", "off", "false", "disable", "0").
func isTUNDisabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "none", "non", "no", "off", "false", "disable", "disabled", "0":
		return true
	default:
		return false
	}
}

// logStartupConfig logs the effective VPN configuration summary from a VPNConfig.
func logStartupConfig(cfg VPNConfig) {
	configItems := []string{
		fmt.Sprintf("api_url=%s", cfg.APIURL),
		fmt.Sprintf("connect_url=%s", cfg.ConnectURL),
		fmt.Sprintf("tun=%s", cfg.TunName),
		fmt.Sprintf("ip_cidr=%s", cfg.IPCIDR),
		fmt.Sprintf("mtu=%d", cfg.MTU),
		fmt.Sprintf("default_route=%t", cfg.DefaultRoute),
	}
	if strings.TrimSpace(cfg.ExtraRoutes) != "" {
		configItems = append(configItems, fmt.Sprintf("route=%s", strings.TrimSpace(cfg.ExtraRoutes)))
	}
	if strings.TrimSpace(cfg.ExcludeRoutes) != "" {
		configItems = append(configItems, fmt.Sprintf("exclude_route=%s", strings.TrimSpace(cfg.ExcludeRoutes)))
	}
	if strings.TrimSpace(cfg.DNSList) != "" {
		configItems = append(configItems, fmt.Sprintf("dns=%s", strings.TrimSpace(cfg.DNSList)))
	}
	if cfg.DNSService != "" {
		configItems = append(configItems, fmt.Sprintf("dns_service=%s", cfg.DNSService))
	}
	if cfg.DNSBootstrap != "" {
		configItems = append(configItems, fmt.Sprintf("dns_bootstrap=%s", cfg.DNSBootstrap))
	}
	if cfg.SOCKSListen != "" {
		configItems = append(configItems, fmt.Sprintf("socks=%s", cfg.SOCKSListen))
	}
	if len(cfg.AllowDomains) > 0 {
		configItems = append(configItems, fmt.Sprintf("domain=%s", strings.Join(cfg.AllowDomains, ",")))
	}
	if len(cfg.ExcludeDomains) > 0 {
		configItems = append(configItems, fmt.Sprintf("exclude_domain=%s", strings.Join(cfg.ExcludeDomains, ",")))
	}
	configItems = append(configItems, fmt.Sprintf("debug=%t", cfg.Debug))
	if strings.TrimSpace(cfg.JWT) != "" {
		configItems = append(configItems, "jwt=provided")
	} else {
		configItems = append(configItems, "jwt=missing")
	}
	logInfo("startup: %s\n", strings.Join(configItems, " "))
}

// (removed legacy userspace filtering helpers)
