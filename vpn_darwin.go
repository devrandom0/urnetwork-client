//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/songgao/water"
)

// cmdVpn (macOS): create a utun device and bridge packets with RemoteUserNatMultiClient.
// Note: You typically need sudo to create/configure utun and set routes.
func cmdVpn(ctx context.Context, cfg VPNConfig) error {
	tunName := cfg.TunName
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")

	logStartupConfig(cfg)

	if cfg.EnableKillSwitch && !cfg.DefaultRoute {
		logWarn("--kill_switch has no effect without --default_route; kill switch requires a full default route to block leaks\n")
	}

	// If TUN is disabled or not specified (and not a missing-arg case), run SOCKS-only.
	if isTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		if cfg.SOCKSListen == "" {
			logError("--tun=none specified but no --socks provided; nothing to do\n")
			return nil
		}
		stopSocks, err := StartSocks5(ctx, cfg.SOCKSListen, "", cfg.Debug, cfg.AllowDomains, cfg.ExcludeDomains, splitCSV(cfg.DNSList))
		if err != nil {
			return fmt.Errorf("start socks failed: %w", err)
		}
		defer func() { _ = stopSocks() }()
		logInfo("SOCKS started without TUN (system routes only). Press Ctrl+C to exit.\n")
		<-ctx.Done()
		return nil
	}

	// Create TUN device.
	waterCfg := water.Config{DeviceType: water.TUN}
	if tunLikelyMissingArg {
		waterCfg.Name = ""
		logWarn("--tun provided without a valid name (got %q); using auto utun\n", rawTun)
	} else {
		waterCfg.Name = tunName
	}
	dev, err := water.New(waterCfg)
	if err != nil {
		if strings.TrimSpace(waterCfg.Name) != "" {
			logWarn("failed to create %s (%v); retrying with auto utun name\n", waterCfg.Name, err)
			waterCfg.Name = ""
			dev, err = water.New(waterCfg)
		}
		if err != nil {
			return fmt.Errorf("create utun failed: %w", err)
		}
	}
	defer func() { _ = dev.Close() }()
	actualName := dev.Name()
	if actualName == "" {
		actualName = tunName
	}
	logInfo("TUN %s created\n", actualName)

	// Derive TUN IP and peer; configure the interface.
	tunIP, peerIP := tunCIDRParts(cfg.IPCIDR)
	_ = runSudo("ifconfig", actualName, "inet", tunIP, peerIP, "mtu", fmt.Sprintf("%d", cfg.MTU), "up")

	// Add IPv6 address to support IPv6 traffic through the VPN.
	// Use a ULA (Unique Local Address) prefix with the same /120 subnet as IPv4.
	_ = runSudo("ifconfig", actualName, "inet6", "fd00::2/120")

	if cfg.SOCKSListen != "" && !cfg.DefaultRoute && cfg.ExtraRoutes == "" && cfg.ExcludeRoutes == "" {
		logInfo("SOCKS mode without route changes: only SOCKS traffic will use the VPN.\n")
	}

	// Packet counters (shared with the DNS cache goroutine).
	var pktsIn, bytesIn, pktsOut, bytesOut uint64

	// Detect original default gateway before altering routes.
	defGw, _, gwErr := getDefaultGateway()
	if gwErr != nil && (cfg.DefaultRoute || strings.TrimSpace(cfg.ExcludeRoutes) != "") {
		logWarn("failed to detect default gateway: %v\n", gwErr)
	}

	// Set up route manager; Cleanup runs on exit via defer.
	rm := newDarwinRouteManager(actualName, peerIP, defGw)
	defer rm.Cleanup()

	// Install routes based on mode.
	if cfg.DefaultRoute {
		if cfg.EnableKillSwitch {
			rm.AddKillSwitchRoute()
		}
		rm.AddBypassEndpoint(cfg.APIURL)
		rm.AddBypassEndpoint(cfg.ConnectURL)
		rm.AddSplitDefault()
		for _, r := range splitCSV(cfg.ExcludeRoutes) {
			rm.AddExclude(r)
		}
	} else if cfg.SOCKSListen != "" {
		if cfg.ExtraRoutes == "" && cfg.ExcludeRoutes == "" {
			rm.AddScopedDefault()
		}
		for _, r := range splitCSV(cfg.ExcludeRoutes) {
			rm.AddScopedExclude(r)
		}
	}
	for _, r := range splitCSV(cfg.ExtraRoutes) {
		rm.AddExtraRoute(r)
	}

	// DNS configuration.
	if cfg.DNSList != "" {
		if cfg.DNSService != "" {
			if err := rm.SetDNS(splitCSV(cfg.DNSList), cfg.DNSService); err != nil {
				logWarn("failed to set DNS via networksetup for %s: %v\n", cfg.DNSService, err)
			}
		} else {
			logWarn("--dns provided without --dns_service; skipping DNS change on macOS\n")
		}
		bypass := cfg.DefaultRoute && (cfg.DNSBootstrap == "bypass" || cfg.DNSBootstrap == "cache")
		rm.AddDNSServerRoutes(splitCSV(cfg.DNSList), bypass)
	} else if cfg.DefaultRoute && defGw != "" && (cfg.DNSBootstrap == "bypass" || cfg.DNSBootstrap == "cache") {
		// No --dns: bypass current system resolvers so DNS works during default-route switch.
		if resolvers, err := getSystemDNSResolvers(); err == nil {
			rm.AddDNSServerRoutes(resolvers, true)
			if len(resolvers) > 0 {
				logInfo("Kept existing DNS resolvers via %s: %v\n", defGw, resolvers)
			}
		} else {
			logWarn("failed to detect system DNS resolvers: %v\n", err)
		}
	}
	if !cfg.DefaultRoute && cfg.DNSList != "" {
		rm.AddDNSServerRoutes(splitCSV(cfg.DNSList), false)
	}

	// DNS cache bootstrap: remove DNS bypass once the tunnel has traffic.
	if cfg.DefaultRoute && cfg.DNSBootstrap == "cache" {
		go func() {
			deadline := time.After(3 * time.Second)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
		loop:
			for {
				select {
				case <-deadline:
					break loop
				case <-ticker.C:
					if atomic.LoadUint64(&pktsIn) > 0 && atomic.LoadUint64(&pktsOut) > 0 {
						break loop
					}
				}
			}
			rm.RemoveDNSBypass()
			logInfo("DNS bootstrap cache complete; DNS bypass removed\n")
		}()
	}

	// Run shared dataplane + SOCKS + stats.
	vpnRunCore(ctx, dev, actualName, cfg, &pktsIn, &pktsOut, &bytesIn, &bytesOut, func() {})
	return nil
}

// tunCIDRParts extracts the host IP and derives a peer/gateway IP from an ip/prefix CIDR.
// For "10.255.0.2/24" it returns ("10.255.0.2", "10.255.0.1").
func tunCIDRParts(ipCIDR string) (ip, peer string) {
	ip = ipCIDR
	peer = "10.255.0.1"
	if idx := strings.Index(ipCIDR, "/"); idx >= 0 {
		ip = ipCIDR[:idx]
	}
	if i := strings.LastIndex(ip, "."); i > 0 {
		peer = ip[:i] + ".1"
	}
	return
}

func runSudo(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getDefaultGateway returns the IPv4 default gateway and interface (e.g., 192.168.1.1, en0) on macOS.
func getDefaultGateway() (string, string, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	// Don't attach Stdout/Stderr to avoid noisy output; capture instead
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("route get default failed: %w", err)
	}
	var gw, iface string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			gw = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		} else if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	if gw == "" {
		return "", "", fmt.Errorf("no default gateway found")
	}
	return gw, iface, nil
}

// getSystemDNSResolvers parses `scutil --dns` and returns unique IPv4 resolver IPs.
func getSystemDNSResolvers() ([]string, error) {
	out, err := runCapture("scutil", "--dns")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	seen := map[string]bool{}
	var res []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		// Lines look like: 'nameserver[0] : 192.168.1.1'
		if strings.HasPrefix(ln, "nameserver[") {
			parts := strings.Split(ln, ":")
			if len(parts) >= 2 {
				ip := strings.TrimSpace(parts[1])
				if ip != "" && strings.Count(ip, ".") == 3 && !seen[ip] {
					seen[ip] = true
					res = append(res, ip)
				}
			}
		}
	}
	return res, nil
}
