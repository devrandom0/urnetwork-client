//go:build darwin

package main

import (
	"context"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/songgao/water"
)

// cmdVpn (macOS): create a utun device and bridge packets with RemoteUserNatMultiClient.
// Note: You typically need sudo to create/configure utun and set routes.
func cmdVpn(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
	// Default: no TUN unless explicitly provided
	tunName := getStringOr(opts, "--tun", "") // Name is advisory on darwin; kernel assigns utunX
	rawTun := strings.TrimSpace(tunName)
	// If user passed `--tun` without a value, docopt may consume the next flag token (e.g., "--default_route").
	// Detect such cases and treat as a request for an auto-assigned utun.
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")
	ipCIDR := getStringOr(opts, "--ip_cidr", "10.255.0.2/24")
	mtu := getIntOr(opts, "--mtu", 1420)
	defRoute, _ := opts.Bool("--default_route")
	localOnly, _ := opts.Bool("--local_only")
	noFwRules, _ := opts.Bool("--no_fw_rules")
	extraRoutes := getStringOr(opts, "--route", "")
	excludeRoutes := getStringOr(opts, "--exclude_route", "")
	dnsList := getStringOr(opts, "--dns", "")
	dnsService := strings.TrimSpace(getStringOr(opts, "--dns_service", ""))
	dnsBootstrap := strings.TrimSpace(getStringOr(opts, "--dns_bootstrap", "bypass")) // bypass|cache|none
	socksListen := strings.TrimSpace(getStringOr(opts, "--socks", getStringOr(opts, "--socks_listen", "")))
	allowDomains := splitCSV(getStringOr(opts, "--domain", ""))
	excludeDomains := splitCSV(getStringOr(opts, "--exclude_domain", ""))
	debugOn, _ := opts.Bool("--debug")
	_ = time.Duration(getIntOr(opts, "--stats_interval", 5)) * time.Second
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// If user disables TUN (e.g., --tun=none/non/no/off/false/disable/0) or omits it (empty and not a missing-arg case),
	// run SOCKS-only without creating a utun.
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

	cfg := water.Config{DeviceType: water.TUN}
	if tunLikelyMissingArg {
		// Auto-assign utun: kernel picks a free name when Name is empty
		cfg.Name = ""
		logWarn("--tun provided without a valid name (got %q); using auto utun\n", rawTun)
	} else {
		cfg.Name = tunName
	}
	dev, err := water.New(cfg)
	if err != nil {
		// If a specific utun name fails (format or in use), retry with auto assignment
		if strings.TrimSpace(cfg.Name) != "" {
			logWarn("failed to create %s (%v); retrying with auto utun name\n", cfg.Name, err)
			cfg.Name = ""
			dev, err = water.New(cfg)
		}
		if err != nil {
			fatal(fmt.Errorf("create utun failed: %w", err))
		}
	}
	defer dev.Close()
	actualName := dev.Name()
	if actualName == "" {
		actualName = tunName
	}
	logInfo("TUN %s created (configure IP/routes/DNS via ifconfig/route as needed)\n", actualName)
	if socksListen != "" && strings.TrimSpace(extraRoutes) == "" && !defRoute && strings.TrimSpace(excludeRoutes) == "" {
		logInfo("SOCKS mode without route changes: only SOCKS traffic will use the VPN.\n")
	}

	// Packet counters (used by stats and dns_bootstrap cache logic)
	var pktsIn, bytesIn, pktsOut, bytesOut uint64

	// Capture current default gateway before we alter routing, so we can bypass excluded prefixes via it
	defGw, _, gwErr := getDefaultGateway()
	if gwErr != nil && strings.TrimSpace(excludeRoutes) != "" {
		logWarn("failed to detect default gateway for exclude routing: %v\n", gwErr)
	}
	type addedRoute struct {
		isHost bool
		dest   string
	}
	var addedExcludes []addedRoute
	// Track control-plane bypass host routes (api/connect endpoints via original gateway)
	var addedCtrlBypass []string
	// Track DNS resolver IPs we bypass via the original gateway when no --dns is provided
	var addedDnsBypass []string
	// Track split-default routes we add so we can clean them up precisely
	type splitAdded struct {
		dest, mask string
		usedCIDR   bool
	}
	var addedSplits []splitAdded

	// Configure IP and MTU
	// ifconfig utunX inet <ip> <peer> mtu <mtu> up
	var peerIP string
	if strings.Contains(ipCIDR, "/") {
		// For simplicity, derive a peer in same /24: peer = .1
		parts := strings.Split(ipCIDR, "/")
		ip := parts[0]
		peer := "10.255.0.1"
		if i := strings.LastIndex(ip, "."); i > 0 {
			peer = ip[:i] + ".1"
		}
		peerIP = peer
		_ = runSudo("ifconfig", actualName, "inet", ip, peer, "mtu", fmt.Sprintf("%d", mtu), "up")
	}

	// Routes: default or specific ones
	if defRoute && !noFwRules {
		// Before changing default routes, ensure we can still reach control-plane endpoints
		if defGw != "" {
			addBypass := func(raw string) {
				if strings.TrimSpace(raw) == "" {
					return
				}
				// Ensure scheme for url.Parse
				u, err := neturl.Parse(raw)
				if err != nil || u.Host == "" {
					// Fallback: treat raw as host
					host := raw
					// Trim any path
					if i := strings.Index(host, "/"); i >= 0 {
						host = host[:i]
					}
					ips, _ := net.LookupIP(host)
					for _, ip := range ips {
						ipv4 := ip.To4()
						if ipv4 == nil {
							continue
						}
						ipStr := ipv4.String()
						if out, err := runCapture("route", "-n", "add", "-host", ipStr, defGw); err == nil || strings.Contains(out, "File exists") {
							if err == nil {
								addedCtrlBypass = append(addedCtrlBypass, ipStr)
							}
						}
					}
					return
				}
				host := u.Host
				// Strip port if present
				if i := strings.Index(host, ":"); i >= 0 {
					host = host[:i]
				}
				ips, _ := net.LookupIP(host)
				for _, ip := range ips {
					ipv4 := ip.To4()
					if ipv4 == nil {
						continue
					}
					ipStr := ipv4.String()
					if out, err := runCapture("route", "-n", "add", "-host", ipStr, defGw); err == nil || strings.Contains(out, "File exists") {
						if err == nil {
							addedCtrlBypass = append(addedCtrlBypass, ipStr)
						}
					}
				}
			}
			addBypass(apiUrl)
			addBypass(connectUrl)
		}
		// Try multiple variants for split default; track exactly what we add for cleanup
		addVariant := func(dest, mask string) bool {
			// 1) -net with -netmask via -interface
			if out, err := runCapture("route", "-n", "add", "-net", dest, "-netmask", mask, "-interface", actualName); err == nil {
				addedSplits = append(addedSplits, splitAdded{dest: dest, mask: mask, usedCIDR: false})
				return true
			} else if strings.Contains(out, "File exists") {
				if _, chErr := runCapture("route", "-n", "change", "-net", dest, "-netmask", mask, "-interface", actualName); chErr == nil {
					addedSplits = append(addedSplits, splitAdded{dest: dest, mask: mask, usedCIDR: false})
					return true
				}
			}
			// 2) CIDR with -interface
			cidr := dest + "/1"
			if out, err := runCapture("route", "-n", "add", "-net", cidr, "-interface", actualName); err == nil {
				addedSplits = append(addedSplits, splitAdded{dest: cidr, mask: "", usedCIDR: true})
				return true
			} else if strings.Contains(out, "File exists") {
				if _, chErr := runCapture("route", "-n", "change", "-net", cidr, "-interface", actualName); chErr == nil {
					addedSplits = append(addedSplits, splitAdded{dest: cidr, mask: "", usedCIDR: true})
					return true
				}
			}
			// 3) -net with -netmask via peer gateway and -ifscope
			if strings.TrimSpace(peerIP) != "" {
				if out, err := runCapture("route", "-n", "add", "-net", dest, "-netmask", mask, peerIP, "-ifscope", actualName); err == nil {
					addedSplits = append(addedSplits, splitAdded{dest: dest, mask: mask, usedCIDR: false})
					return true
				} else if strings.Contains(out, "File exists") {
					if _, chErr := runCapture("route", "-n", "change", "-net", dest, "-netmask", mask, peerIP, "-ifscope", actualName); chErr == nil {
						addedSplits = append(addedSplits, splitAdded{dest: dest, mask: mask, usedCIDR: false})
						return true
					}
				}
				// 4) CIDR via peer gateway and -ifscope
				if out, err := runCapture("route", "-n", "add", "-net", cidr, peerIP, "-ifscope", actualName); err == nil {
					addedSplits = append(addedSplits, splitAdded{dest: cidr, mask: "", usedCIDR: true})
					return true
				} else if strings.Contains(out, "File exists") {
					if _, chErr := runCapture("route", "-n", "change", "-net", cidr, peerIP, "-ifscope", actualName); chErr == nil {
						addedSplits = append(addedSplits, splitAdded{dest: cidr, mask: "", usedCIDR: true})
						return true
					}
				}
			}
			logWarn("failed to add split default for %s (%s)\n", dest, mask)
			return false
		}
		_ = addVariant("0.0.0.0", "128.0.0.0")
		_ = addVariant("128.0.0.0", "128.0.0.0")
		if strings.TrimSpace(excludeRoutes) != "" {
			for _, r := range splitCSV(excludeRoutes) {
				isHost := !strings.Contains(r, "/")
				// Try adding via original gateway first, fall back to reject on failure or no gateway
				added := false
				if defGw != "" {
					if isHost {
						if out, err := runCapture("route", "-n", "add", "-host", r, defGw); err == nil {
							addedExcludes = append(addedExcludes, addedRoute{isHost: true, dest: r})
							added = true
						} else if !strings.Contains(out, "File exists") {
							// On other errors, try reject fallback
							if out2, err2 := runCapture("route", "-n", "add", "-host", r, "-reject"); err2 == nil {
								addedExcludes = append(addedExcludes, addedRoute{isHost: true, dest: r})
								added = true
							} else if !strings.Contains(out2, "File exists") {
								logWarn("failed to add exclude host %s: %v\n", r, err2)
							}
						}
					} else {
						if out, err := runCapture("route", "-n", "add", "-net", r, defGw); err == nil {
							addedExcludes = append(addedExcludes, addedRoute{isHost: false, dest: r})
							added = true
						} else if !strings.Contains(out, "File exists") {
							if out2, err2 := runCapture("route", "-n", "add", "-net", r, "-reject"); err2 == nil {
								addedExcludes = append(addedExcludes, addedRoute{isHost: false, dest: r})
								added = true
							} else if !strings.Contains(out2, "File exists") {
								logWarn("failed to add exclude net %s: %v\n", r, err2)
							}
						}
					}
				} else {
					// No gateway known; try reject route
					if isHost {
						if out, err := runCapture("route", "-n", "add", "-host", r, "-reject"); err == nil {
							addedExcludes = append(addedExcludes, addedRoute{isHost: true, dest: r})
							added = true
						} else if !strings.Contains(out, "File exists") {
							logWarn("failed to add exclude host %s: %v\n", r, err)
						}
					} else {
						if out, err := runCapture("route", "-n", "add", "-net", r, "-reject"); err == nil {
							addedExcludes = append(addedExcludes, addedRoute{isHost: false, dest: r})
							added = true
						} else if !strings.Contains(out, "File exists") {
							logWarn("failed to add exclude net %s: %v\n", r, err)
						}
					}
				}
				_ = added // placeholder; kept for clarity though not used after appending
			}
		}
	}
	// SOCKS-only scoped routing: when SOCKS is enabled but no global route flags,
	// install split defaults scoped to utun so only SOCKS-bound sockets use them.
	if socksListen != "" && !defRoute && !noFwRules && strings.TrimSpace(extraRoutes) == "" && strings.TrimSpace(excludeRoutes) == "" {
		// Reuse the addVariant helper style locally
		addScoped := func(dest, mask string) {
			// Prefer peer next-hop with -ifscope, then fall back to -interface CIDR
			if strings.TrimSpace(peerIP) != "" {
				if out, err := runCapture("route", "-n", "add", "-net", dest, "-netmask", mask, peerIP, "-ifscope", actualName); err == nil || strings.Contains(out, "File exists") {
					if err == nil {
						addedSplits = append(addedSplits, splitAdded{dest: dest, mask: mask, usedCIDR: false})
					}
					// Local-only: prevent acting as exit node by rejecting forwarding via utun.
					// We add interface-scoped reject for default so forwarding via utun fails locally.
					if localOnly {
						if strings.TrimSpace(peerIP) != "" {
							_ = runSudo("route", "-n", "add", "-net", "0.0.0.0", "-netmask", "0.0.0.0", "-reject", "-ifscope", actualName)
						} else {
							_ = runSudo("route", "-n", "add", "-net", "0.0.0.0/0", "-reject", "-ifscope", actualName)
						}
					}
				}
			} else {
				cidr := dest + "/1"
				if out, err := runCapture("route", "-n", "add", "-net", cidr, "-ifscope", actualName); err == nil || strings.Contains(out, "File exists") {
					if err == nil {
						addedSplits = append(addedSplits, splitAdded{dest: cidr, mask: "", usedCIDR: true})
					}
				}
			}
		}
		addScoped("0.0.0.0", "128.0.0.0")
		addScoped("128.0.0.0", "128.0.0.0")
	}
	// Scoped excludes for SOCKS-only mode: keep certain prefixes off the utun
	if socksListen != "" && !defRoute && !noFwRules && strings.TrimSpace(excludeRoutes) != "" {
		for _, r := range splitCSV(excludeRoutes) {
			if strings.Contains(r, "/") {
				_ = runSudo("route", "-n", "add", "-net", r, "-reject", "-ifscope", actualName)
			} else {
				_ = runSudo("route", "-n", "add", "-host", r, "-reject", "-ifscope", actualName)
			}
		}
	}
	if strings.TrimSpace(extraRoutes) != "" && !noFwRules {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			// Use correct forms: host/net with -interface; fallback to peer + -ifscope if needed
			var out string
			var err error
			if strings.Contains(r, "/") {
				out, err = runCapture("route", "-n", "add", "-net", r, "-interface", actualName)
			} else {
				out, err = runCapture("route", "-n", "add", "-host", r, "-interface", actualName)
			}
			if err != nil && !strings.Contains(out, "File exists") {
				if strings.TrimSpace(peerIP) != "" {
					if strings.Contains(r, "/") {
						out, err = runCapture("route", "-n", "add", "-net", r, peerIP, "-ifscope", actualName)
					} else {
						out, err = runCapture("route", "-n", "add", "-host", r, peerIP, "-ifscope", actualName)
					}
				}
			}
			if err != nil && !strings.Contains(out, "File exists") {
				logWarn("failed to add extra route %s: %v\n", r, err)
			}
		}
	}

	// DNS configuration (system-wide for a Network Service)
	var dnsConfigured bool
	if strings.TrimSpace(dnsList) != "" && dnsService != "" && !noFwRules {
		servers := splitCSV(dnsList)
		if len(servers) > 0 {
			args := append([]string{"-setdnsservers", dnsService}, servers...)
			if err := runSudo("networksetup", args...); err != nil {
				logWarn("failed to set DNS via networksetup for %s: %v\n", dnsService, err)
			} else {
				dnsConfigured = true
				logInfo("DNS set for service %s -> %v\n", dnsService, servers)
			}
		}
	} else if strings.TrimSpace(dnsList) != "" && dnsService == "" {
		logWarn("--dns provided without --dns_service; skipping DNS change on macOS\n")
	}

	// If not default route, ensure DNS servers route via utun so queries prefer tunnel
	if !defRoute && strings.TrimSpace(dnsList) != "" && !noFwRules {
		for _, d := range splitCSV(dnsList) {
			if strings.Contains(d, "/") {
				_ = runSudo("route", "-n", "add", d, "-interface", actualName)
			} else {
				_ = runSudo("route", "-n", "add", "-host", d, "-interface", actualName)
			}
		}
	}
	// If default route is enabled, try to keep DNS working by ensuring resolvers are reachable via the original gateway.
	// Case A: No --dns provided -> use current system resolvers from scutil
	if defRoute && !noFwRules && strings.TrimSpace(dnsList) == "" && defGw != "" && (dnsBootstrap == "bypass" || dnsBootstrap == "cache") {
		if resolvers, err := getSystemDNSResolvers(); err == nil {
			for _, ip := range resolvers {
				if ip == "" {
					continue
				}
				if out, err := runCapture("route", "-n", "add", "-host", ip, defGw); err == nil || strings.Contains(out, "File exists") {
					if err == nil {
						addedDnsBypass = append(addedDnsBypass, ip)
					}
				}
			}
			if len(addedDnsBypass) > 0 {
				logInfo("Kept existing DNS resolvers via %s: %v\n", defGw, addedDnsBypass)
			}
		} else {
			logWarn("failed to detect system DNS resolvers: %v\n", err)
		}
	}
	// Case B: --dns provided -> also add bypass routes to those DNS IPs via original gateway so we don't break bootstrap
	if defRoute && !noFwRules && strings.TrimSpace(dnsList) != "" && defGw != "" && (dnsBootstrap == "bypass" || dnsBootstrap == "cache") {
		for _, ip := range splitCSV(dnsList) {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if strings.Contains(ip, "/") {
				continue
			} // expect IPs only
			if out, err := runCapture("route", "-n", "add", "-host", ip, defGw); err == nil || strings.Contains(out, "File exists") {
				if err == nil {
					addedDnsBypass = append(addedDnsBypass, ip)
				}
			}
		}
		if len(addedDnsBypass) > 0 {
			logInfo("DNS servers via original gateway %s: %v\n", defGw, addedDnsBypass)
		}
	}

	// If cache mode: after tunnel sends/receives some traffic, remove DNS bypass so DNS goes through VPN only
	if defRoute && !noFwRules && (dnsBootstrap == "cache") {
		go func() {
			// wait until some packets in and out or a brief delay, whichever first
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
			// Remove DNS bypass routes so subsequent DNS uses VPN
			for _, ip := range addedDnsBypass {
				_ = runSudo("route", "-n", "delete", "-host", ip)
			}
			addedDnsBypass = nil
			logInfo("DNS bootstrap cache complete; DNS bypass removed\n")
		}()
	}

	// Run shared dataplane + SOCKS + stats; cancel ctx on SIGINT/SIGTERM in background
	go waitForInterrupt(cancel)
	vpnRunCore(ctx, dev, actualName, opts, jwt, &pktsIn, &pktsOut, &bytesIn, &bytesOut, func() {})

	// After core returns, perform OS-specific cleanup
	if defRoute && !noFwRules {
		// Remove whichever split-default variant(s) we added
		if len(addedSplits) > 0 {
			for _, s := range addedSplits {
				if s.usedCIDR {
					_ = runSudo("route", "-n", "delete", "-net", s.dest)
				} else {
					_ = runSudo("route", "-n", "delete", "-net", s.dest, "-netmask", s.mask)
				}
			}
		} else {
			// Fallback cleanup if tracking wasn't populated
			_ = runSudo("route", "-n", "delete", "-net", "0.0.0.0/1")
			_ = runSudo("route", "-n", "delete", "-net", "128.0.0.0/1")
			_ = runSudo("route", "-n", "delete", "-net", "0.0.0.0", "-netmask", "128.0.0.0")
			_ = runSudo("route", "-n", "delete", "-net", "128.0.0.0", "-netmask", "128.0.0.0")
		}
		if len(addedExcludes) > 0 {
			for _, ar := range addedExcludes {
				if ar.isHost {
					_ = runSudo("route", "-n", "delete", "-host", ar.dest)
				} else {
					_ = runSudo("route", "-n", "delete", "-net", ar.dest)
				}
			}
		}
		if len(addedCtrlBypass) > 0 {
			for _, ip := range addedCtrlBypass {
				_ = runSudo("route", "-n", "delete", "-host", ip)
			}
		}
		if len(addedDnsBypass) > 0 {
			for _, ip := range addedDnsBypass {
				_ = runSudo("route", "-n", "delete", "-host", ip)
			}
		}
	}
	if strings.TrimSpace(extraRoutes) != "" && !noFwRules {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			_ = runSudo("route", "-n", "delete", r)
		}
	}
	if socksListen != "" && !defRoute && !noFwRules && strings.TrimSpace(excludeRoutes) != "" {
		for _, r := range splitCSV(excludeRoutes) {
			if strings.Contains(r, "/") {
				_ = runSudo("route", "-n", "delete", "-net", r)
			} else {
				_ = runSudo("route", "-n", "delete", "-host", r)
			}
		}
	}
	if !defRoute && strings.TrimSpace(dnsList) != "" && !noFwRules {
		for _, d := range splitCSV(dnsList) {
			if strings.Contains(d, "/") {
				_ = runSudo("route", "-n", "delete", d)
			} else {
				_ = runSudo("route", "-n", "delete", "-host", d)
			}
		}
	}
	if dnsConfigured && !noFwRules {
		// Clear DNS servers
		if err := runSudo("networksetup", "-setdnsservers", dnsService, "Empty"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: failed to clear DNS for %s: %v\n", dnsService, err)
		}
	}
	if localOnly && !noFwRules {
		// Remove the reject route if we added it
		_ = runSudo("route", "-n", "delete", "-net", "0.0.0.0", "-netmask", "0.0.0.0")
		_ = runSudo("route", "-n", "delete", "-net", "0.0.0.0/0")
	}
	_ = runSudo("ifconfig", actualName, "down")
}

func runSudo(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// splitCSV is implemented in util.go (shared)

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

// runCapture executes a command and returns combined stdout/stderr and an error.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
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
