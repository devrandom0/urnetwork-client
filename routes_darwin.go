//go:build darwin

package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// darwinAddedRoute records a route entry with type metadata for precise cleanup.
type darwinAddedRoute struct {
	isHost bool   // true = -host, false = -net
	dest   string // destination (host address or CIDR)
}

// darwinSplitRoute records a successfully added split-default route variant.
type darwinSplitRoute struct {
	dest     string // destination address or CIDR (includes prefix when usedCIDR=true)
	mask     string // netmask (empty when usedCIDR=true)
	usedCIDR bool   // true = added with CIDR form; false = -net/-netmask form
}

// darwinRouteManager implements RouteManager for macOS using `route` and `networksetup`.
// All route additions are recorded so Cleanup can reverse them precisely.
type darwinRouteManager struct {
	tunName string // TUN interface name (e.g. utun3)
	peerIP  string // peer/gateway IP for the utun (derived from --ip_cidr)
	defGw   string // original default gateway before VPN changes

	addedCtrlBypass  []string           // IPs given bypass host routes for API/connect endpoints
	addedDNSBypass   []string           // IPs given bypass host routes for DNS servers
	addedExcludes    []darwinAddedRoute // exclude routes via defGw or reject
	addedScopedExcls []darwinAddedRoute // scoped reject excludes (SOCKS-only mode)
	addedSplits      []darwinSplitRoute // split-default routes that were successfully added
	addedExtra       []darwinAddedRoute // explicit routes through TUN from --route
	addedDNSTun      []darwinAddedRoute // DNS server routes through TUN (!defRoute mode)

	killSwitch      bool // whether kill-switch mode is active
	killSwitchAdded bool // whether the blackhole default was successfully installed

	dnsConfigured bool   // whether SetDNS changed the system DNS
	dnsService    string // network service name passed to networksetup
}

// newDarwinRouteManager creates a route manager for the given TUN interface.
// peerIP is the peer/gateway address for the utun (derived from --ip_cidr).
// defGw is the pre-VPN default gateway (empty string is tolerated).
func newDarwinRouteManager(tunName, peerIP, defGw string) *darwinRouteManager {
	return &darwinRouteManager{
		tunName: tunName,
		peerIP:  peerIP,
		defGw:   defGw,
	}
}

// AddBypassEndpoint resolves the hostname in rawURL and installs bypass host routes
// for each resolved IPv4 address via the original default gateway.
func (m *darwinRouteManager) AddBypassEndpoint(rawURL string) {
	if m.defGw == "" {
		return
	}
	host := extractHost(rawURL)
	if host == "" {
		return
	}
	ips, _ := net.LookupIP(host)
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		ipStr := v4.String()
		if out, err := runCapture("route", "-n", "add", "-host", ipStr, m.defGw); err == nil || strings.Contains(out, "File exists") {
			if err == nil {
				m.addedCtrlBypass = append(m.addedCtrlBypass, ipStr)
			}
		}
	}
}

// AddSplitDefault installs 0.0.0.0/1 and 128.0.0.0/1 through the TUN.
// Multiple route command variants are tried to handle different macOS versions.
func (m *darwinRouteManager) AddSplitDefault() {
	m.addVariant("0.0.0.0", "128.0.0.0")
	m.addVariant("128.0.0.0", "128.0.0.0")
}

// AddScopedDefault installs split-default routes scoped to the TUN (SOCKS-only mode).
// Only SOCKS-bound sockets will use the TUN; system traffic keeps its normal path.
func (m *darwinRouteManager) AddScopedDefault() {
	m.addScoped("0.0.0.0", "128.0.0.0")
	m.addScoped("128.0.0.0", "128.0.0.0")
}

// AddScopedExclude installs a scoped reject route for dest (SOCKS-only mode).
func (m *darwinRouteManager) AddScopedExclude(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	isHost := !strings.Contains(dest, "/")
	var err error
	if isHost {
		_, err = runCapture("route", "-n", "add", "-host", dest, "-reject", "-ifscope", m.tunName)
	} else {
		_, err = runCapture("route", "-n", "add", "-net", dest, "-reject", "-ifscope", m.tunName)
	}
	if err == nil {
		m.addedScopedExcls = append(m.addedScopedExcls, darwinAddedRoute{isHost: isHost, dest: dest})
	}
}

// AddExclude installs a route for dest that bypasses the TUN.
// Prefers routing via the original default gateway; falls back to a reject route.
func (m *darwinRouteManager) AddExclude(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	isHost := !strings.Contains(dest, "/")
	if m.defGw != "" {
		var out string
		var err error
		if isHost {
			out, err = runCapture("route", "-n", "add", "-host", dest, m.defGw)
		} else {
			out, err = runCapture("route", "-n", "add", "-net", dest, m.defGw)
		}
		if err == nil {
			m.addedExcludes = append(m.addedExcludes, darwinAddedRoute{isHost: isHost, dest: dest})
			return
		}
		if strings.Contains(out, "File exists") {
			return
		}
		// Fall through to reject on other errors.
	}
	// No gateway or primary attempt failed: install a reject route.
	var out string
	var err error
	if isHost {
		out, err = runCapture("route", "-n", "add", "-host", dest, "-reject")
	} else {
		out, err = runCapture("route", "-n", "add", "-net", dest, "-reject")
	}
	if err == nil {
		m.addedExcludes = append(m.addedExcludes, darwinAddedRoute{isHost: isHost, dest: dest})
	} else if !strings.Contains(out, "File exists") {
		logWarn("failed to add exclude %s: %v\n", dest, err)
	}
}

// AddExtraRoute installs an explicit route for dest through the TUN.
func (m *darwinRouteManager) AddExtraRoute(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	isHost := !strings.Contains(dest, "/")
	var out string
	var err error
	if isHost {
		out, err = runCapture("route", "-n", "add", "-host", dest, "-interface", m.tunName)
	} else {
		out, err = runCapture("route", "-n", "add", "-net", dest, "-interface", m.tunName)
	}
	if err != nil && !strings.Contains(out, "File exists") && m.peerIP != "" {
		if isHost {
			out, err = runCapture("route", "-n", "add", "-host", dest, m.peerIP, "-ifscope", m.tunName)
		} else {
			out, err = runCapture("route", "-n", "add", "-net", dest, m.peerIP, "-ifscope", m.tunName)
		}
	}
	if err != nil && !strings.Contains(out, "File exists") {
		logWarn("failed to add extra route %s: %v\n", dest, err)
		return
	}
	m.addedExtra = append(m.addedExtra, darwinAddedRoute{isHost: isHost, dest: dest})
}

// AddDNSServerRoutes installs routes for the given DNS server IPs.
// bypass=true routes them via the original gateway; bypass=false routes through the TUN.
func (m *darwinRouteManager) AddDNSServerRoutes(ips []string, bypass bool) {
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || strings.Contains(ip, "/") {
			continue // DNS entries must be bare IPs
		}
		if bypass {
			if m.defGw == "" {
				continue
			}
			if out, err := runCapture("route", "-n", "add", "-host", ip, m.defGw); err == nil || strings.Contains(out, "File exists") {
				if err == nil {
					m.addedDNSBypass = append(m.addedDNSBypass, ip)
				}
			}
		} else {
			if _, err := runCapture("route", "-n", "add", "-host", ip, "-interface", m.tunName); err == nil {
				m.addedDNSTun = append(m.addedDNSTun, darwinAddedRoute{isHost: true, dest: ip})
			}
		}
	}
}

// SetDNS configures system-wide DNS resolvers via networksetup.
func (m *darwinRouteManager) SetDNS(servers []string, service string) error {
	if service == "" || len(servers) == 0 {
		return nil
	}
	args := append([]string{"-setdnsservers", service}, servers...)
	if err := runSudo("networksetup", args...); err != nil {
		return err
	}
	m.dnsConfigured = true
	m.dnsService = service
	logInfo("DNS set for service %s -> %v\n", service, servers)
	return nil
}

// RemoveDNSBypass removes all DNS server bypass routes immediately.
// Used by the dns_bootstrap=cache goroutine once the VPN tunnel has traffic.
func (m *darwinRouteManager) RemoveDNSBypass() {
	for _, ip := range m.addedDNSBypass {
		_ = runSudo("route", "-n", "delete", "-host", ip)
	}
	m.addedDNSBypass = nil
}

// AddKillSwitchRoute installs a blackhole default route so that if the VPN split
// routes are removed, all traffic is blocked rather than leaking via the real
// default gateway. Call this before AddSplitDefault so the /1 routes take priority.
// The route is left in place on Cleanup when kill-switch mode is active.
func (m *darwinRouteManager) AddKillSwitchRoute() {
	m.killSwitch = true
	// Replace the existing default with a blackhole so traffic is blocked
	// when the VPN split routes are absent. The /1 split routes are more
	// specific and will supersede this while the VPN is running.
	if m.defGw != "" {
		_ = runSudo("route", "-n", "delete", "default")
	}
	if _, err := runCapture("route", "-n", "add", "-blackhole", "default"); err == nil {
		m.killSwitchAdded = true
		logInfo("kill switch: blackhole default route installed\n")
	} else {
		logWarn("kill switch: failed to install blackhole default route; leak protection may be incomplete\n")
	}
}

// Cleanup removes all routes and DNS configuration applied by this manager
// and brings the TUN interface down.
func (m *darwinRouteManager) Cleanup() {
	// Split-default routes
	if len(m.addedSplits) > 0 {
		for _, s := range m.addedSplits {
			if s.usedCIDR {
				_ = runSudo("route", "-n", "delete", "-net", s.dest)
			} else {
				_ = runSudo("route", "-n", "delete", "-net", s.dest, "-netmask", s.mask)
			}
		}
	}
	// Exclude routes
	for _, ar := range m.addedExcludes {
		if ar.isHost {
			_ = runSudo("route", "-n", "delete", "-host", ar.dest)
		} else {
			_ = runSudo("route", "-n", "delete", "-net", ar.dest)
		}
	}
	// Scoped excludes (SOCKS-only mode)
	for _, ar := range m.addedScopedExcls {
		if ar.isHost {
			_ = runSudo("route", "-n", "delete", "-host", ar.dest)
		} else {
			_ = runSudo("route", "-n", "delete", "-net", ar.dest)
		}
	}
	// Control-plane bypass routes
	for _, ip := range m.addedCtrlBypass {
		_ = runSudo("route", "-n", "delete", "-host", ip)
	}
	// DNS bypass routes (may already be nil if RemoveDNSBypass was called)
	for _, ip := range m.addedDNSBypass {
		_ = runSudo("route", "-n", "delete", "-host", ip)
	}
	// Extra TUN routes
	for _, ar := range m.addedExtra {
		if ar.isHost {
			_ = runSudo("route", "-n", "delete", "-host", ar.dest)
		} else {
			_ = runSudo("route", "-n", "delete", "-net", ar.dest)
		}
	}
	// DNS through-TUN routes
	for _, ar := range m.addedDNSTun {
		if ar.isHost {
			_ = runSudo("route", "-n", "delete", "-host", ar.dest)
		} else {
			_ = runSudo("route", "-n", "delete", "-net", ar.dest)
		}
	}
	// DNS service
	if m.dnsConfigured {
		if err := runSudo("networksetup", "-setdnsservers", m.dnsService, "Empty"); err != nil {
			fmt.Fprintf(os.Stderr, "warn: failed to clear DNS for %s: %v\n", m.dnsService, err)
		}
	}
	// Bring interface down
	_ = runSudo("ifconfig", m.tunName, "down")
	// Kill switch: keep blackhole in place (traffic stays blocked after VPN exits).
	// Without kill switch: remove blackhole and restore original default gateway.
	if m.killSwitchAdded {
		if m.killSwitch {
			logInfo("kill switch: blackhole default route preserved; all traffic is blocked until you run: sudo route delete default && sudo route add default <gateway>\n")
		} else {
			_ = runSudo("route", "-n", "delete", "default")
			if m.defGw != "" {
				_ = runSudo("route", "-n", "add", "default", m.defGw)
			}
		}
	}
}

// addVariant tries multiple route command variants to install a split-default entry.
// The first variant that succeeds is recorded in addedSplits for cleanup.
func (m *darwinRouteManager) addVariant(dest, mask string) bool {
	cidr := dest + "/1"

	// Variant 1: -net/-netmask via -interface
	if out, err := runCapture("route", "-n", "add", "-net", dest, "-netmask", mask, "-interface", m.tunName); err == nil {
		m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: dest, mask: mask})
		return true
	} else if strings.Contains(out, "File exists") {
		if _, chErr := runCapture("route", "-n", "change", "-net", dest, "-netmask", mask, "-interface", m.tunName); chErr == nil {
			m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: dest, mask: mask})
			return true
		}
	}

	// Variant 2: CIDR form via -interface
	if out, err := runCapture("route", "-n", "add", "-net", cidr, "-interface", m.tunName); err == nil {
		m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: cidr, usedCIDR: true})
		return true
	} else if strings.Contains(out, "File exists") {
		if _, chErr := runCapture("route", "-n", "change", "-net", cidr, "-interface", m.tunName); chErr == nil {
			m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: cidr, usedCIDR: true})
			return true
		}
	}

	if m.peerIP == "" {
		logWarn("failed to add split default for %s (%s)\n", dest, mask)
		return false
	}

	// Variant 3: -net/-netmask via peer + -ifscope
	if out, err := runCapture("route", "-n", "add", "-net", dest, "-netmask", mask, m.peerIP, "-ifscope", m.tunName); err == nil {
		m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: dest, mask: mask})
		return true
	} else if strings.Contains(out, "File exists") {
		if _, chErr := runCapture("route", "-n", "change", "-net", dest, "-netmask", mask, m.peerIP, "-ifscope", m.tunName); chErr == nil {
			m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: dest, mask: mask})
			return true
		}
	}

	// Variant 4: CIDR form via peer + -ifscope
	if out, err := runCapture("route", "-n", "add", "-net", cidr, m.peerIP, "-ifscope", m.tunName); err == nil {
		m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: cidr, usedCIDR: true})
		return true
	} else if strings.Contains(out, "File exists") {
		if _, chErr := runCapture("route", "-n", "change", "-net", cidr, m.peerIP, "-ifscope", m.tunName); chErr == nil {
			m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: cidr, usedCIDR: true})
			return true
		}
	}

	logWarn("failed to add split default for %s (%s)\n", dest, mask)
	return false
}

// addScoped installs a split-default route scoped to the TUN interface (SOCKS-only mode).
func (m *darwinRouteManager) addScoped(dest, mask string) {
	if m.peerIP != "" {
		out, err := runCapture("route", "-n", "add", "-net", dest, "-netmask", mask, m.peerIP, "-ifscope", m.tunName)
		if err == nil || strings.Contains(out, "File exists") {
			if err == nil {
				m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: dest, mask: mask})
			}
			return
		}
	}
	// Fallback: CIDR form with -ifscope
	cidr := dest + "/1"
	if out, err := runCapture("route", "-n", "add", "-net", cidr, "-ifscope", m.tunName); err == nil || strings.Contains(out, "File exists") {
		if err == nil {
			m.addedSplits = append(m.addedSplits, darwinSplitRoute{dest: cidr, usedCIDR: true})
		}
	}
}
