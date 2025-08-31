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
	"time"

	"github.com/docopt/docopt-go"
	"github.com/songgao/water"
)

func cmdVpn(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
	// Default: no TUN unless explicitly provided
	tunName := getStringOr(opts, "--tun", "")
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")
	ipCIDR := getStringOr(opts, "--ip_cidr", "10.255.0.2/24")
	mtu := getIntOr(opts, "--mtu", 1420)
	defRoute, _ := opts.Bool("--default_route")
	localOnly, _ := opts.Bool("--local_only")
	noFwRules, _ := opts.Bool("--no_fw_rules")
	denySrcList := strings.TrimSpace(getStringOr(opts, "--deny_forward_src", ""))
	allowSrcList := strings.TrimSpace(getStringOr(opts, "--allow_forward_src", ""))
	extraRoutes := getStringOr(opts, "--route", "")
	excludeRoutes := getStringOr(opts, "--exclude_route", "")
	dnsList := getStringOr(opts, "--dns", "")
	debugOn, _ := opts.Bool("--debug")
	socksListen := strings.TrimSpace(getStringOr(opts, "--socks", getStringOr(opts, "--socks_listen", "")))
	allowDomains := splitCSV(getStringOr(opts, "--domain", ""))
	excludeDomains := splitCSV(getStringOr(opts, "--exclude_domain", ""))
	_ = time.Duration(getIntOr(opts, "--stats_interval", 5)) * time.Second
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TUN-less mode for SOCKS-only usage (none/non/no/off/false/disable/0 or empty and not a missing-arg case)
	if func(n string) bool {
		s := strings.ToLower(strings.TrimSpace(n))
		switch s {
		case "none", "non", "no", "off", "false", "disable", "disabled", "0":
			return true
		default:
			return false
		}
	}(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		if socksListen == "" {
			logError("--tun=none specified but no --socks provided; nothing to do\n")
			return
		}
		stopSocks, err := StartSocks5(ctx, socksListen, "", debugOn, allowDomains, excludeDomains)
		if err != nil {
			fatal(fmt.Errorf("start socks failed: %w", err))
		}
		defer stopSocks()
		logInfo("SOCKS started without TUN (system routes only). Press Ctrl+C to exit.\n")
		waitForInterrupt(cancel)
		return
	}

	// If tun name looks like a flag (missing value), choose a sane default name
	if tunLikelyMissingArg {
		logWarn("--tun provided without a valid name (got %q); using default 'urnet0'\n", rawTun)
		tunName = "urnet0"
	}

	cfg := water.Config{DeviceType: water.TUN}
	cfg.Name = tunName
	dev, err := water.New(cfg)
	if err != nil {
		fatal(fmt.Errorf("create TUN failed: %w", err))
	}
	defer dev.Close()
	logInfo("TUN %s created (configuring IP/MTU/routes as requested)\n", tunName)

	// Configure IP and MTU
	_ = run("ip", "addr", "add", ipCIDR, "dev", tunName)
	_ = run("ip", "link", "set", "dev", tunName, "mtu", fmt.Sprintf("%d", mtu))
	_ = run("ip", "link", "set", tunName, "up")

	// If requested, harden against acting as an exit node on Linux.
	// Best-effort steps:
	// 1) Disable IPv4 forwarding during the session.
	// 2) Add iptables filter rules to drop FORWARD traffic via the TUN interface.
	if localOnly && !noFwRules {
		_ = run("sh", "-c", "sysctl -w net.ipv4.ip_forward=0 >/dev/null 2>&1 || true")
		_ = run("iptables", "-I", "FORWARD", "-i", tunName, "-j", "DROP")
		_ = run("iptables", "-I", "FORWARD", "-o", tunName, "-j", "DROP")
	}
	// If allowlist is provided, enforce default DROP for forwarding via TUN, then allow listed sources.
	var allowRules [][8]string
	var allowDropInstalled bool
	if allowSrcList != "" && !noFwRules {
		// Install DROP first
		_ = run("iptables", "-I", "FORWARD", "-o", tunName, "-j", "DROP")
		_ = run("iptables", "-I", "FORWARD", "-i", tunName, "-j", "DROP")
		allowDropInstalled = true
		for _, cidr := range strings.Split(allowSrcList, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			// Allow forwarding of packets with source in cidr to go out via TUN
			_ = run("iptables", "-I", "FORWARD", "-s", cidr, "-o", tunName, "-j", "ACCEPT")
			allowRules = append(allowRules, [8]string{"FORWARD", "-s", cidr, "-o", tunName, "ACCEPT", "", ""})
			// And allow return traffic from TUN destined to that subnet
			_ = run("iptables", "-I", "FORWARD", "-i", tunName, "-d", cidr, "-j", "ACCEPT")
			allowRules = append(allowRules, [8]string{"FORWARD", "-i", tunName, "-d", cidr, "ACCEPT", "", ""})
		}
	}
	// Deny specific source networks from being forwarded via the VPN (router B clients).
	var denyRules [][6]string
	if denySrcList != "" && !noFwRules {
		for _, cidr := range strings.Split(denySrcList, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			// Drop any packet entering from those sources heading out via TUN
			_ = run("iptables", "-I", "FORWARD", "-s", cidr, "-o", tunName, "-j", "DROP")
			denyRules = append(denyRules, [6]string{"FORWARD", "-s", cidr, "-o", tunName, "DROP"})
			// And drop packets entering from TUN destined to those sources on LAN (both directions)
			_ = run("iptables", "-I", "FORWARD", "-i", tunName, "-d", cidr, "-j", "DROP")
			denyRules = append(denyRules, [6]string{"FORWARD", "-i", tunName, "-d", cidr, "DROP"})
		}
	}

	// Routes: default or extra
	var origGw, origDev string
	var addedBypass []string
	var addedExclude []string
	if defRoute && !noFwRules {
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
		addBypass(apiUrl)
		addBypass(connectUrl)
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
	if strings.TrimSpace(extraRoutes) != "" && !noFwRules {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			_ = run("ip", "route", "add", r, "dev", tunName)
		}
	}
	if !defRoute && strings.TrimSpace(dnsList) != "" && !noFwRules {
		for _, d := range strings.Split(dnsList, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			_ = run("ip", "route", "add", d, "dev", tunName)
		}
	}

	// Run shared dataplane + SOCKS + stats
	var pktsIn, bytesIn, pktsOut, bytesOut uint64
	go waitForInterrupt(cancel)
	vpnRunCore(ctx, dev, tunName, opts, jwt, &pktsIn, &pktsOut, &bytesIn, &bytesOut, func() {})

	// Cleanup on exit: delete routes, bring link down, and delete address
	if defRoute && !noFwRules {
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
	if strings.TrimSpace(extraRoutes) != "" && !noFwRules {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			_ = run("ip", "route", "del", r)
		}
	}
	if !defRoute && strings.TrimSpace(dnsList) != "" && !noFwRules {
		for _, d := range strings.Split(dnsList, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			_ = run("ip", "route", "del", d)
		}
	}
	if localOnly && !noFwRules {
		// Remove iptables rules we inserted. Use -D to delete matching rules.
		_ = run("iptables", "-D", "FORWARD", "-o", tunName, "-j", "DROP")
		_ = run("iptables", "-D", "FORWARD", "-i", tunName, "-j", "DROP")
	}
	if len(denyRules) > 0 && !noFwRules {
		// Delete in reverse order
		for i := len(denyRules) - 1; i >= 0; i-- {
			r := denyRules[i]
			// r layout: chain, x1, v1, x2, v2, target
			_ = run("iptables", "-D", r[0], r[1], r[2], r[3], r[4], "-j", r[5])
		}
	}
	if len(allowRules) > 0 && !noFwRules {
		for i := len(allowRules) - 1; i >= 0; i-- {
			r := allowRules[i]
			// chain, -s/-i, v, -o/-d, v, target
			_ = run("iptables", "-D", r[0], r[1], r[2], r[3], r[4], "-j", r[5])
		}
	}
	if allowDropInstalled && !noFwRules {
		_ = run("iptables", "-D", "FORWARD", "-i", tunName, "-j", "DROP")
		_ = run("iptables", "-D", "FORWARD", "-o", tunName, "-j", "DROP")
	}
	_ = run("ip", "link", "set", tunName, "down")
	_ = run("ip", "addr", "flush", "dev", tunName)
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

// runCapture runs a command and returns stdout as string.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(b), nil
}
