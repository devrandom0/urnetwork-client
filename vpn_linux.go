//go:build linux

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/songgao/water"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

func cmdVpn(ctx context.Context, cfg VPNConfig) error {
	tunName := cfg.TunName
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")

	logStartupConfig(cfg)

	if cfg.EnableKillSwitch && !cfg.DefaultRoute {
		logx.Warn("--kill_switch has no effect without --default_route; kill switch requires a full default route to block leaks\n")
	}

	// TUN-less mode: SOCKS-only when TUN is disabled or not specified.
	if isTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		return runSocksOnly(ctx, cfg)
	}

	// If tun name looks like a flag (missing value), use a safe default.
	if tunLikelyMissingArg {
		logx.Warn("--tun provided without a valid name (got %q); using default 'urnet0'\n", rawTun)
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
	logx.Info("TUN %s created\n", tunName)

	if err := configureLinuxTUN(tunName, cfg); err != nil {
		return err
	}

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
			if err := rm.AddKillSwitchRoute(); err != nil {
				return err
			}
		}
		rm.AddBypassEndpoint(cfg.APIURL)
		rm.AddBypassEndpoint(cfg.ConnectURL)
		if err := rm.AddSplitDefault(); err != nil {
			return err
		}
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
	return cmdRunner.Run(name, args...)
}

// configureLinuxTUN must succeed before any route points at the TUN; a half-configured
// device would blackhole all routed traffic.
func configureLinuxTUN(name string, cfg VPNConfig) error {
	steps := [][]string{
		{"ip", "addr", "add", cfg.IPCIDR, "dev", name},
		{"ip", "link", "set", "dev", name, "mtu", strconv.Itoa(cfg.MTU)},
		{"ip", "link", "set", name, "up"},
	}
	for _, s := range steps {
		if err := run(s[0], s[1:]...); err != nil {
			return fmt.Errorf("configure TUN %s (%s): %w", name, strings.Join(s, " "), err)
		}
	}
	if err := run("ip", "addr", "add", "fd00::2/120", "dev", name); err != nil {
		if cfg.EnableIPv6 {
			return fmt.Errorf("configure TUN %s IPv6 address: %w", name, err)
		}
		logx.Debug("IPv6 address on %s not set (%v); continuing because --enable_ipv6 is off\n", name, err)
	}
	return nil
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
