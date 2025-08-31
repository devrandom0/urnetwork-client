package main

import (
	"context"
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

	// Build provider specs based on location flags (shared logic)
	strat := connect.NewClientStrategyWithDefaults(ctx)
	specs := []*connect.ProviderSpec{}
	if id := stringsTrim(getStringOr(opts, "--location_id", "")); id != "" {
		if loc, err := connect.ParseId(id); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationId: &loc})
		}
	}
	if gid := stringsTrim(getStringOr(opts, "--location_group_id", "")); gid != "" {
		if lg, err := connect.ParseId(gid); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationGroupId: &lg})
		}
	}
	if len(specs) == 0 {
		if q := stringsTrim(getStringOr(opts, "--location_query", "")); q != "" {
			if httpRes, err := httpFindLocations(ctx, apiUrl, jwt, q); err == nil && httpRes != nil && len(httpRes.Specs) > 0 {
				specs = httpRes.Specs
				logInfo("using %d specs from location query: %s\n", len(specs), q)
			}
			if len(specs) == 0 {
				fb := findSpecsByQueryFallback(ctx, strat, apiUrl, jwt, q)
				if len(fb) > 0 {
					specs = fb
					logInfo("using %d specs from provider-locations (fallback) for: %s\n", len(specs), q)
				}
			}
		}
	}
	if len(specs) == 0 {
		specs = []*connect.ProviderSpec{{BestAvailable: true}}
	}
	appVer := fmt.Sprintf("urnet-client %s", Version)
	gen := connect.NewApiMultiClientGeneratorWithDefaults(
		ctx, specs, strat, nil, apiUrl, jwt, fmt.Sprintf("%s/", connectUrl), "", "", appVer, nil,
	)

	// Precompute userspace filtering configuration if requested; used by both directions
	noFwRules, _ := opts.Bool("--no_fw_rules")
	localOnly, _ := opts.Bool("--local_only")
	allowSrcList := stringsTrim(getStringOr(opts, "--allow_forward_src", ""))
	denySrcList := stringsTrim(getStringOr(opts, "--deny_forward_src", ""))
	var allowCIDRs []*net.IPNet
	var denyCIDRs []*net.IPNet
	// Capture local IPv4 addresses set for local_only userspace enforcement
	localIPv4 := map[string]struct{}{}
	if noFwRules {
		if allowSrcList != "" {
			for _, s := range splitCSV(allowSrcList) {
				if c := parseCIDR(s); c != nil { allowCIDRs = append(allowCIDRs, c) }
			}
		}
		if denySrcList != "" {
			for _, s := range splitCSV(denySrcList) {
				if c := parseCIDR(s); c != nil { denyCIDRs = append(denyCIDRs, c) }
			}
		}
		if localOnly {
			// Snapshot local IPv4s
			ifs, _ := net.Interfaces()
			for _, inf := range ifs {
				addrs, _ := inf.Addrs()
				for _, a := range addrs {
					var ip net.IP
					switch v := a.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}
					if v4 := ip.To4(); v4 != nil {
						localIPv4[v4.String()] = struct{}{}
					}
				}
			}
		}
	}

	// Provider receive: optional userspace filtering, then write to TUN and update counters
	receive := func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		if debugOn || isDebugEnabled() {
			logInfo("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		}
		if noFwRules && localOnly {
			// Drop if destination isn't a local interface IP (prevents forwarding to other LAN hosts)
			if len(packet) >= 20 && (packet[0]>>4) == 4 { // IPv4 only
				ihl := int(packet[0]&0x0F) * 4
				if ihl >= 20 && len(packet) >= ihl {
					dst := net.IPv4(packet[16], packet[17], packet[18], packet[19])
					if _, ok := localIPv4[dst.String()]; !ok {
						if debugOn || isDebugEnabled() { logInfo("dropped inbound by userspace local_only filter: dst=%s\n", dst.String()) }
						return
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
			// Optional userspace filtering
			if noFwRules {
				if dropPacketUserspace(pkt, localOnly, allowCIDRs, denyCIDRs, localIPv4) {
					if debugOn || isDebugEnabled() { logInfo("dropped by userspace filter\n") }
					continue
				}
			}
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

// waitForInterrupt blocks until SIGINT or SIGTERM, then calls cancel.
func waitForInterrupt(cancel context.CancelFunc) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	<-sigc
	cancel()
}

// parseCIDR returns a *net.IPNet for a CIDR string or single IPv4 host.
func parseCIDR(s string) *net.IPNet {
	s = strings.TrimSpace(s)
	if s == "" { return nil }
	if !strings.Contains(s, "/") {
		// Treat as /32
		if ip := net.ParseIP(s); ip != nil {
			ip4 := ip.To4()
			if ip4 == nil { return nil }
			return &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}
		}
		return nil
	}
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil { return nil }
	if ip4 := ip.To4(); ip4 != nil {
		return &net.IPNet{IP: ip4, Mask: ipnet.Mask}
	}
	return nil
}

// dropPacketUserspace inspects an IPv4 packet and decides whether to drop it based on:
// - localOnly: only permit packets whose source is a local interface IP
// - allowCIDRs: if non-empty, only allow sources within these ranges
// - denyCIDRs: drop if source is within these ranges
// Returns true when the packet should be dropped.
func dropPacketUserspace(pkt []byte, localOnly bool, allowCIDRs, denyCIDRs []*net.IPNet, localIPv4 map[string]struct{}) bool {
	if len(pkt) < 20 { return false }
	// IPv4 only
	if (pkt[0] >> 4) != 4 { return false }
	ihl := int(pkt[0]&0x0F) * 4
	if ihl < 20 || len(pkt) < ihl { return false }
	src := net.IPv4(pkt[12], pkt[13], pkt[14], pkt[15])
	// local_only: only allow packets originating from local interface addresses
	if localOnly {
		if _, ok := localIPv4[src.String()]; !ok {
			return true
		}
	}
	// deny list first
	for _, n := range denyCIDRs {
		if n != nil && n.Contains(src) { return true }
	}
	// allow list: if present, require membership
	if len(allowCIDRs) > 0 {
		allowed := false
		for _, n := range allowCIDRs {
			if n != nil && n.Contains(src) { allowed = true; break }
		}
		if !allowed { return true }
	}
	return false
}
