//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/songgao/water"
)

func cmdVpn(ctx context.Context, cfg VPNConfig) error {
	tunName := cfg.TunName
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")

	logStartupConfig(cfg)

	if cfg.EnableKillSwitch && !cfg.DefaultRoute {
		logWarn("--kill_switch has no effect without --default_route; kill switch requires a full default route to block leaks\n")
	}

	// TUN-less mode: SOCKS-only when TUN is disabled or not specified.
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

	// If tun name looks like a flag (missing value), use a safe default.
	if tunLikelyMissingArg {
		logWarn("--tun provided without a valid name (got %q); using default 'urnet0'\n", rawTun)
		tunName = "urnet0"
	}

	// Create TUN device.
	waterCfg := water.Config{DeviceType: water.TUN}
	waterCfg.Name = tunName
	dev, err := water.New(waterCfg)
	if err != nil {
		return fmt.Errorf("create TUN failed: %w", err)
	}
	defer func() { _ = dev.Close() }()
	logInfo("TUN %s created\n", tunName)

	// Configure IP address and MTU.
	_ = run("ip", "addr", "add", cfg.IPCIDR, "dev", tunName)
	_ = run("ip", "link", "set", "dev", tunName, "mtu", strconv.Itoa(cfg.MTU))
	_ = run("ip", "link", "set", tunName, "up")

	// Add IPv6 address to support IPv6 traffic through the VPN.
	// Use a ULA (Unique Local Address) prefix with /120 subnet.
	_ = run("ip", "addr", "add", "fd00::2/120", "dev", tunName)

	// Detect current default gateway for bypass and exclude routing.
	origGw, origDev := "", ""
	if routes, err := linuxListDefaultRoutes(); err == nil {
		for _, r := range routes {
			if r.Dev == tunName {
				continue
			}
			// Prefer the route that has a gateway.
			if origDev == "" || (origGw == "" && r.Gw != "") {
				origGw, origDev = r.Gw, r.Dev
			}
		}
	}

	// Set up route manager; Cleanup runs on exit via defer.
	rm := newLinuxRouteManager(tunName, origGw, origDev)
	defer rm.Cleanup()

	// Install routes.
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
	}
	for _, r := range splitCSV(cfg.ExtraRoutes) {
		rm.AddExtraRoute(r)
	}
	if !cfg.DefaultRoute && cfg.DNSList != "" {
		rm.AddDNSServerRoutes(splitCSV(cfg.DNSList), false)
	}

	// Run shared dataplane + SOCKS + stats.
	var pktsIn, bytesIn, pktsOut, bytesOut uint64
	vpnRunCore(ctx, dev, tunName, cfg, &pktsIn, &pktsOut, &bytesIn, &bytesOut, func() {})
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// linuxListDefaultRoutes parses all current default routes via `ip -o route show default`.
type defaultRoute struct {
	Gw, Dev string
	Metric  int
}

func linuxListDefaultRoutes() ([]defaultRoute, error) {
	out, err := runCapture("ip", "-o", "route", "show", "default")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	routes := []defaultRoute{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		toks := strings.Fields(line)
		var gw, dev string
		met := -1
		for i := 0; i < len(toks); i++ {
			switch toks[i] {
			case "via":
				if i+1 < len(toks) {
					gw = toks[i+1]
					i++
				}
			case "dev":
				if i+1 < len(toks) {
					dev = toks[i+1]
					i++
				}
			case "metric":
				if i+1 < len(toks) {
					if v, e := strconv.Atoi(toks[i+1]); e == nil {
						met = v
					}
					i++
				}
			}
		}
		if dev == "" {
			continue
		}
		routes = append(routes, defaultRoute{Gw: gw, Dev: dev, Metric: met})
	}
	return routes, nil
}
