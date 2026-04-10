//go:build linux

package main

import (
	"context"
	"fmt"
	"net"
	neturl "net/url"
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
	ipCIDR := cfg.IPCIDR
	mtu := cfg.MTU
	defRoute := cfg.DefaultRoute
	extraRoutes := cfg.ExtraRoutes
	excludeRoutes := cfg.ExcludeRoutes
	dnsList := cfg.DNSList
	debugOn := cfg.Debug
	connectURL := cfg.ConnectURL
	socksListen := cfg.SOCKSListen
	allowDomains := cfg.AllowDomains
	excludeDomains := cfg.ExcludeDomains

	logStartupConfig(cfg)

	// TUN-less mode for SOCKS-only usage
	if isTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		if socksListen == "" {
			logError("--tun=none specified but no --socks provided; nothing to do\n")
			return nil
		}
		stopSocks, err := StartSocks5(ctx, socksListen, "", debugOn, allowDomains, excludeDomains)
		if err != nil {
			return fmt.Errorf("start socks failed: %w", err)
		}
		defer func() { _ = stopSocks() }()
		logInfo("SOCKS started without TUN (system routes only). Press Ctrl+C to exit.\n")
		<-ctx.Done()
		return nil
	}

	// If tun name looks like a flag (missing value), choose a sane default name
	if tunLikelyMissingArg {
		logWarn("--tun provided without a valid name (got %q); using default 'urnet0'\n", rawTun)
		tunName = "urnet0"
	}

	waterCfg := water.Config{DeviceType: water.TUN}
	waterCfg.Name = tunName
	dev, err := water.New(waterCfg)
	if err != nil {
		return fmt.Errorf("create TUN failed: %w", err)
	}
	defer func() { _ = dev.Close() }()
	logInfo("TUN %s created (configuring IP/MTU/routes as requested)\n", tunName)

	// Configure IP and MTU
	_ = run("ip", "addr", "add", ipCIDR, "dev", tunName)
	_ = run("ip", "link", "set", "dev", tunName, "mtu", fmt.Sprintf("%d", mtu))
	_ = run("ip", "link", "set", tunName, "up")

	// Reverted forwarding/iptables controls; no local_only/allow/deny/no_fw_rules logic here.

	// Routes: default or extra
	var origGw, origDev string
	var addedBypass []string
	var addedExclude []string
	if defRoute {
		// macOS-like split default: add two /1 routes via the TUN so default traffic prefers it.
		// 1) Determine current default route (for bypass/excludes)
		if routes, err := linuxListDefaultRoutes(); err == nil {
			for _, r := range routes {
				if r.Dev == tunName {
					continue
				}
				// prefer route with gateway
				if origDev == "" || (origGw == "" && r.Gw != "") {
					origGw, origDev = r.Gw, r.Dev
				}
			}
		}
		// 2) Add control-plane bypass host routes (API and connect endpoints) via original gateway/dev
		addBypass := func(raw string) {
			raw = strings.TrimSpace(raw)
			if raw == "" || origDev == "" {
				return
			}
			u, err := neturl.Parse(raw)
			var host string
			if err == nil && u.Host != "" {
				host = u.Host
				if i := strings.Index(host, ":"); i >= 0 {
					host = host[:i]
				}
			} else {
				host = raw
				if i := strings.Index(host, "/"); i >= 0 {
					host = host[:i]
				}
			}
			ips, _ := net.LookupIP(host)
			for _, ip := range ips {
				v4 := ip.To4()
				if v4 == nil {
					continue
				}
				ipStr := v4.String()
				if origGw != "" {
					_ = run("ip", "route", "add", ipStr, "via", origGw, "dev", origDev)
				} else {
					_ = run("ip", "route", "add", ipStr, "dev", origDev)
				}
				addedBypass = append(addedBypass, ipStr)
			}
		}
		addBypass(cfg.APIURL)
		addBypass(connectURL)
		// 3) Add split default to send all non-bypass traffic via TUN
		_ = run("ip", "route", "add", "0.0.0.0/1", "dev", tunName)
		_ = run("ip", "route", "add", "128.0.0.0/1", "dev", tunName)
		// 4) Optional excludes: route them via original gateway or reject (unreachable)
		if strings.TrimSpace(excludeRoutes) != "" {
			for _, r := range strings.Split(excludeRoutes, ",") {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				if origDev != "" && origGw != "" {
					_ = run("ip", "route", "add", r, "via", origGw, "dev", origDev)
					addedExclude = append(addedExclude, r)
				} else if origDev != "" {
					_ = run("ip", "route", "add", r, "dev", origDev)
					addedExclude = append(addedExclude, r)
				} else {
					_ = run("ip", "route", "add", r, "unreachable")
					addedExclude = append(addedExclude, r)
				}
			}
		}
	}
	if strings.TrimSpace(extraRoutes) != "" {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			_ = run("ip", "route", "add", r, "dev", tunName)
		}
	}
	if !defRoute && strings.TrimSpace(dnsList) != "" {
		for _, d := range strings.Split(dnsList, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			_ = run("ip", "route", "add", d, "dev", tunName)
		}
	}

	// Run shared dataplane + SOCKS + stats; ctx cancelled by signal (from main's NotifyContext).
	var pktsIn, bytesIn, pktsOut, bytesOut uint64
	vpnRunCore(ctx, dev, tunName, cfg, &pktsIn, &pktsOut, &bytesIn, &bytesOut, func() {})

	// Cleanup on exit: delete routes, bring link down, and delete address
	if defRoute {
		// Remove split defaults
		_ = run("ip", "route", "del", "0.0.0.0/1")
		_ = run("ip", "route", "del", "128.0.0.0/1")
		// Remove added bypass and exclude routes
		for _, ip := range addedBypass {
			_ = run("ip", "route", "del", ip)
		}
		for _, r := range addedExclude {
			_ = run("ip", "route", "del", r)
		}
	}
	if strings.TrimSpace(extraRoutes) != "" {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			_ = run("ip", "route", "del", r)
		}
	}
	if !defRoute && strings.TrimSpace(dnsList) != "" {
		for _, d := range strings.Split(dnsList, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			_ = run("ip", "route", "del", d)
		}
	}
	_ = run("ip", "link", "set", tunName, "down")
	_ = run("ip", "addr", "flush", "dev", tunName)
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
