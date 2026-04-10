package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/songgao/water"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/connect/protocol"
)

// vpnRunCore encapsulates the common dataplane loop, provider selection, stats, and SOCKS handling.
// It waits for termination and then invokes onBeforeExit for OS-specific cleanup.
func vpnRunCore(
	ctx context.Context,
	dev *water.Interface,
	tunIfName string,
	opts docopt.Opts,
	jwt string,
	pktsIn, pktsOut, bytesIn, bytesOut *uint64,
	onBeforeExit func(),
) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
	debugOn, _ := opts.Bool("--debug")
	statsInt := time.Duration(getIntOr(opts, "--stats_interval", 5)) * time.Second

	// Build provider specs from location flags (--location_id / --location_group_id / --location_query).
	strat, specs := buildProviderSpecs(ctx, apiUrl, jwt, opts)
	appVer := fmt.Sprintf("urnet-client %s", Version)
	gen := connect.NewApiMultiClientGeneratorWithDefaults(
		ctx, specs, strat, nil, apiUrl, jwt, fmt.Sprintf("%s/", connectUrl), "", "", appVer, nil,
	)

	// New userspace control: drop new inbound connections when allowlists are provided.
	// Behavior: if --allow_inbound_src or --allow_inbound_local is set, block new inbound TCP SYNs
	// and allow only sources matching those allowlists. Additionally, drop inbound TCP segments
	// without the ACK flag (e.g., stray RST) since they cannot belong to an established flow we initiated.
	allowInboundSrcList := stringsTrim(getStringOr(opts, "--allow_inbound_src", ""))
	allowInboundLocal, _ := opts.Bool("--allow_inbound_local")
	blockNewInbound := allowInboundLocal || (allowInboundSrcList != "")
	var allowInboundCIDRs []*net.IPNet
	if allowInboundSrcList != "" {
		for _, s := range splitCSV(allowInboundSrcList) {
			if n := parseCIDRHost(s); n != nil {
				allowInboundCIDRs = append(allowInboundCIDRs, n)
			}
		}
	}
	// If allow-local requested, append local ranges and the TUN subnet (if provided)
	if allowInboundLocal {
		appendNet := func(cidr string) {
			if n := parseCIDRHost(cidr); n != nil {
				allowInboundCIDRs = append(allowInboundCIDRs, n)
			}
		}
		// Common local ranges
		appendNet("127.0.0.0/8")    // loopback
		appendNet("169.254.0.0/16") // link-local
		appendNet("10.0.0.0/8")     // RFC1918
		appendNet("172.16.0.0/12")  // RFC1918
		appendNet("192.168.0.0/16") // RFC1918
		appendNet("100.64.0.0/10")  // CGNAT
		// TUN subnet from --ip_cidr, if provided
		if cidr := stringsTrim(getStringOr(opts, "--ip_cidr", "")); cidr != "" {
			if strings.Contains(cidr, "/") {
				if n := parseCIDRHost(cidr); n != nil {
					allowInboundCIDRs = append(allowInboundCIDRs, n)
				}
			} else {
				// Single host provided; treat as /32
				if n := parseCIDRHost(cidr); n != nil {
					allowInboundCIDRs = append(allowInboundCIDRs, n)
				}
			}
		}
	}

	if blockNewInbound && isInfoEnabled() {
		logInfo("inbound-control: enabled (allowlist=%d entries); policy: drop new inbound SYN not in allowlist, and drop inbound TCP without ACK\n", len(allowInboundCIDRs))
	}

	// Provider receive: optional userspace filtering, then write to TUN and update counters
	receive := func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		if debugOn || isDebugEnabled() {
			logInfo("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		}
		// If enabled: drop new inbound TCP connections (SYN set, ACK not set)
		if blockNewInbound {
			if len(packet) >= 20 && (packet[0]>>4) == 4 { // IPv4
				ihl := int(packet[0]&0x0F) * 4
				if ihl >= 20 && len(packet) >= ihl+20 { // ensure room for TCP header
					proto := packet[9]
					if proto == 6 { // TCP
						// TCP header starts at ihl; flags are at offset 13 of TCP header
						tcpFlags := packet[ihl+13]
						syn := tcpFlags&0x02 != 0
						ack := tcpFlags&0x10 != 0
						// 1) Drop new inbound SYN if not allowlisted
						if syn && !ack {
							// If allowlist is provided, permit new inbound from those sources
							if len(allowInboundCIDRs) > 0 {
								srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
								allowed := false
								for _, n := range allowInboundCIDRs {
									if n != nil && n.Contains(srcIP) {
										allowed = true
										break
									}
								}
								if allowed {
									// pass through
								} else {
									if debugOn || isDebugEnabled() {
										dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19])
										srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
										dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
										logInfo("dropped new inbound TCP SYN %s:%d -> %s:%d (not in allowlist)\n", srcIP.String(), srcPort, dstIP.String(), dstPort)
									}
									return
								}
							} else {
								if debugOn || isDebugEnabled() {
									srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
									dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19])
									srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
									dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
									logInfo("dropped new inbound TCP SYN %s:%d -> %s:%d\n", srcIP.String(), srcPort, dstIP.String(), dstPort)
								}
								return
							}
							// 2) Drop any inbound TCP segment without ACK (e.g., stray RST), since it can't be part of a valid response path
						} else if !ack {
							srcIP := net.IPv4(packet[12], packet[13], packet[14], packet[15])
							dstIP := net.IPv4(packet[16], packet[17], packet[18], packet[19])
							srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
							dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
							if debugOn || isDebugEnabled() {
								logInfo("dropped inbound TCP without ACK %s:%d -> %s:%d (flags=0x%02x)\n", srcIP.String(), srcPort, dstIP.String(), dstPort, tcpFlags)
							}
							return
						}
					}
				}
			}
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
			if debugOn || isDebugEnabled() {
				logInfo("-> provider len=%d\n", len(pkt))
			}
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
			for range t.C {
				inP := atomic.LoadUint64(pktsIn)
				inB := atomic.LoadUint64(bytesIn)
				outP := atomic.LoadUint64(pktsOut)
				outB := atomic.LoadUint64(bytesOut)
				logInfo("[stats] in=%d pkts / %d bytes, out=%d pkts / %d bytes\n", inP, inB, outP, outB)
			}
		}()
	}

	// Optional SOCKS5 proxy bound to the VPN interface
	socksListen := stringsTrim(getStringOr(opts, "--socks", getStringOr(opts, "--socks_listen", "")))
	allowDomains := splitCSV(getStringOr(opts, "--domain", ""))
	excludeDomains := splitCSV(getStringOr(opts, "--exclude_domain", ""))
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

// tiny helper to avoid repeated TrimSpace everywhere
func stringsTrim(s string) string { return strings.TrimSpace(s) }

// parseCIDRHost parses a CIDR or single IPv4 host into *net.IPNet. Returns nil on failure or non-IPv4.
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
		ip4 := ip.To4()
		if ip4 == nil {
			return nil
		}
		return &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}
	}
	ip, ipn, err := net.ParseCIDR(s)
	if err != nil {
		return nil
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}
	return &net.IPNet{IP: ip4, Mask: ipn.Mask}
}

// waitForInterrupt blocks until SIGINT or SIGTERM, then calls cancel.
func waitForInterrupt(cancel context.CancelFunc) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	<-sigc
	cancel()
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

// logStartupConfig logs the effective VPN configuration summary derived from opts.
// Reading directly from opts makes this safe to call from any platform's cmdVpn before
// TUN creation.
func logStartupConfig(opts docopt.Opts, jwt string) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
	tunName := getStringOr(opts, "--tun", "")
	ipCIDR := getStringOr(opts, "--ip_cidr", "10.255.0.2/24")
	mtu := getIntOr(opts, "--mtu", 1420)
	defRoute, _ := opts.Bool("--default_route")
	extraRoutes := getStringOr(opts, "--route", "")
	excludeRoutes := getStringOr(opts, "--exclude_route", "")
	dnsList := getStringOr(opts, "--dns", "")
	dnsService := strings.TrimSpace(getStringOr(opts, "--dns_service", ""))
	dnsBootstrap := strings.TrimSpace(getStringOr(opts, "--dns_bootstrap", ""))
	socksListen := strings.TrimSpace(getStringOr(opts, "--socks", getStringOr(opts, "--socks_listen", "")))
	allowDomains := splitCSV(getStringOr(opts, "--domain", ""))
	excludeDomains := splitCSV(getStringOr(opts, "--exclude_domain", ""))
	debugOn, _ := opts.Bool("--debug")

	cfg := []string{
		fmt.Sprintf("api_url=%s", apiUrl),
		fmt.Sprintf("connect_url=%s", connectUrl),
		fmt.Sprintf("tun=%s", tunName),
		fmt.Sprintf("ip_cidr=%s", ipCIDR),
		fmt.Sprintf("mtu=%d", mtu),
		fmt.Sprintf("default_route=%t", defRoute),
	}
	if strings.TrimSpace(extraRoutes) != "" {
		cfg = append(cfg, fmt.Sprintf("route=%s", strings.TrimSpace(extraRoutes)))
	}
	if strings.TrimSpace(excludeRoutes) != "" {
		cfg = append(cfg, fmt.Sprintf("exclude_route=%s", strings.TrimSpace(excludeRoutes)))
	}
	if strings.TrimSpace(dnsList) != "" {
		cfg = append(cfg, fmt.Sprintf("dns=%s", strings.TrimSpace(dnsList)))
	}
	if dnsService != "" {
		cfg = append(cfg, fmt.Sprintf("dns_service=%s", dnsService))
	}
	if dnsBootstrap != "" {
		cfg = append(cfg, fmt.Sprintf("dns_bootstrap=%s", dnsBootstrap))
	}
	if socksListen != "" {
		cfg = append(cfg, fmt.Sprintf("socks=%s", socksListen))
	}
	if len(allowDomains) > 0 {
		cfg = append(cfg, fmt.Sprintf("domain=%s", strings.Join(allowDomains, ",")))
	}
	if len(excludeDomains) > 0 {
		cfg = append(cfg, fmt.Sprintf("exclude_domain=%s", strings.Join(excludeDomains, ",")))
	}
	cfg = append(cfg, fmt.Sprintf("debug=%t", debugOn))
	if strings.TrimSpace(jwt) != "" {
		cfg = append(cfg, "jwt=provided")
	} else {
		cfg = append(cfg, "jwt=missing")
	}
	logInfo("startup: %s\n", strings.Join(cfg, " "))
}

// (removed legacy userspace filtering helpers)
