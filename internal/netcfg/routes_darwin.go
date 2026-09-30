//go:build darwin

package netcfg

import (
	"fmt"
	"net/netip"
	"strings"
	"sync"

	"github.com/devrandom0/urnetwork-client/internal/logx"
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

// DarwinRouteManager implements RouteManager for macOS using `route`.
// All route additions are recorded so Cleanup can reverse them precisely.
type DarwinRouteManager struct {
	tunName string // TUN interface name (e.g. utun3)
	peerIP  string // peer/gateway IP for the utun (derived from --ip_cidr)
	defGw   string // original default gateway before VPN changes

	addedCtrlBypass  []string           // IPs given bypass host routes for API/connect endpoints
	addedDNSBypass   []string           // IPs given bypass host routes for DNS servers
	mu               sync.Mutex         // guards addedDNSBypass; RemoveDNSBypass runs on the bootstrap goroutine
	addedExcludes    []darwinAddedRoute // exclude routes via defGw or reject
	addedScopedExcls []darwinAddedRoute // scoped reject excludes (SOCKS-only mode)
	addedSplits      []darwinSplitRoute // split-default routes that were successfully added
	addedExtra       []darwinAddedRoute // explicit routes through TUN from --route
	addedDNSTun      []darwinAddedRoute // DNS server routes through TUN (!defRoute mode)
	addedV6          []string           // IPv6 halves (::/1, 8000::/1) we installed, reject or via TUN

	killSwitch        bool // whether kill-switch mode is active
	killSwitchAdded   bool // whether the blackhole default was successfully installed
	killSwitchV6Added bool // whether the IPv6 blackhole default was installed
}

var _ RouteManager = (*DarwinRouteManager)(nil)

// NewDarwinRouteManager creates a route manager for the given TUN interface.
// peerIP is the peer/gateway address for the utun (derived from --ip_cidr).
// defGw is the pre-VPN default gateway (empty string is tolerated).
func NewDarwinRouteManager(tunName, peerIP, defGw string) *DarwinRouteManager {
	return &DarwinRouteManager{
		tunName: tunName,
		peerIP:  peerIP,
		defGw:   defGw,
	}
}

// AddBypassEndpoint routes the IPv4 addresses of rawURL's host around the tunnel via the
// original gateway and returns the host and the addresses that have such a route, so the DNS
// stub can answer for it without the tunnel.
func (m *DarwinRouteManager) AddBypassEndpoint(rawURL string) (string, []netip.Addr) {
	if m.defGw == "" {
		return "", nil
	}
	host := extractHost(rawURL)
	if host == "" {
		return "", nil
	}
	ips, _ := lookupIP(host)
	var routed []netip.Addr
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		ipStr := v4.String()
		out, err := runCapture("route", "-n", "add", "-host", ipStr, m.defGw)
		if err != nil && !strings.Contains(out, "File exists") {
			logx.Warn("bypass route for %s not installed: %v\n", ipStr, err)
			continue
		}
		if err == nil {
			m.addedCtrlBypass = append(m.addedCtrlBypass, ipStr)
		}
		addr, _ := netip.AddrFromSlice(v4)
		routed = append(routed, addr)
	}
	return host, routed
}

// AddSplitDefault installs 0.0.0.0/1 and 128.0.0.0/1 through the TUN.
// Multiple route command variants are tried to handle different macOS versions.
func (m *DarwinRouteManager) AddSplitDefault() error {
	var failed []string
	for _, dest := range []string{"0.0.0.0", "128.0.0.0"} {
		if !m.addVariant(dest, "128.0.0.0") {
			failed = append(failed, dest+"/1")
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("split default route %s via %s not installed; traffic would leak outside the tunnel", strings.Join(failed, ", "), m.tunName)
	}
	return nil
}

// AddScopedDefault installs split-default routes scoped to the TUN (SOCKS-only mode).
// Only SOCKS-bound sockets will use the TUN; system traffic keeps its normal path.
func (m *DarwinRouteManager) AddScopedDefault() {
	m.addScoped("0.0.0.0", "128.0.0.0")
	m.addScoped("128.0.0.0", "128.0.0.0")
}

// AddScopedExclude installs a scoped reject route for dest (SOCKS-only mode).
func (m *DarwinRouteManager) AddScopedExclude(dest string) {
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
func (m *DarwinRouteManager) AddExclude(dest string) {
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
		logx.Warn("failed to add exclude %s: %v\n", dest, err)
	}
}

// AddExtraRoute installs an explicit route for dest through the TUN.
func (m *DarwinRouteManager) AddExtraRoute(dest string) {
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
		logx.Warn("failed to add extra route %s: %v\n", dest, err)
		return
	}
	m.addedExtra = append(m.addedExtra, darwinAddedRoute{isHost: isHost, dest: dest})
}

// AddDNSServerRoutes installs routes for the given DNS server IPs.
// bypass=true routes them via the original gateway; bypass=false routes through the TUN.
func (m *DarwinRouteManager) AddDNSServerRoutes(ips []string, bypass bool) {
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
					m.mu.Lock()
					m.addedDNSBypass = append(m.addedDNSBypass, ip)
					m.mu.Unlock()
				}
			}
		} else {
			if _, err := runCapture("route", "-n", "add", "-host", ip, "-interface", m.tunName); err == nil {
				m.addedDNSTun = append(m.addedDNSTun, darwinAddedRoute{isHost: true, dest: ip})
			}
		}
	}
}

// RemoveDNSBypass removes all DNS server bypass routes immediately.
// Called from the dns_bootstrap=cache goroutine and from Cleanup; the mutex makes
// the two safe to run concurrently and ensures each route is deleted exactly once.
func (m *DarwinRouteManager) RemoveDNSBypass() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ip := range m.addedDNSBypass {
		_ = runSudo("route", "-n", "delete", "-host", ip)
	}
	m.addedDNSBypass = nil
}

// AddKillSwitchRoute installs a blackhole default route so that if the VPN split
// routes are removed, all traffic is blocked rather than leaking via the real
// default gateway. Call this before AddSplitDefault so the /1 routes take priority.
// The route is left in place on Cleanup when kill-switch mode is active.
func (m *DarwinRouteManager) AddKillSwitchRoute() error {
	m.killSwitch = true
	if err := m.addKillSwitchV6(); err != nil {
		return err
	}
	// Replace the existing default with a blackhole so traffic is blocked
	// when the VPN split routes are absent. The /1 split routes are more
	// specific and will supersede this while the VPN is running.
	deletedDefault := false
	if m.defGw != "" {
		deletedDefault = runSudo("route", "-n", "delete", "default") == nil
	}
	_, err := runCapture("route", "-n", "add", "-blackhole", "default")
	if err == nil {
		m.killSwitchAdded = true
		logx.Info("kill switch: blackhole default route installed\n")
		return nil
	}
	if !deletedDefault {
		return fmt.Errorf("kill switch: install blackhole default route: %w", err)
	}
	if rErr := runSudo("route", "-n", "add", "default", m.defGw); rErr != nil {
		logx.Error("kill switch: could not restore default route via %s: %v; run: sudo route add default %s\n", m.defGw, rErr, m.defGw)
		return fmt.Errorf("kill switch: install blackhole default route: %w; restoring the default route also failed, run: sudo route add default %s", err, m.defGw)
	}
	return fmt.Errorf("kill switch: install blackhole default route: %w; original default route restored", err)
}

// addKillSwitchV6 replaces the IPv6 default with a blackhole. macOS keeps one default route
// per family, so an existing router default has to go first; a later router advertisement,
// or turning the interface off and on, brings it back after the blackhole is removed.
func (m *DarwinRouteManager) addKillSwitchV6() error {
	add := []string{"-n", "add", "-inet6", "-blackhole", "default", "::1"}
	out, err := runCapture("route", add...)
	if err != nil && strings.Contains(out, "File exists") {
		_ = runSudo("route", "-n", "delete", "-inet6", "default")
		_, err = runCapture("route", add...)
	}
	if err != nil {
		return fmt.Errorf("kill switch: install IPv6 blackhole default route: %w", err)
	}
	m.killSwitchV6Added = true
	return nil
}

// AddIPv6Reject makes every IPv6 destination outside connected and link-local prefixes fail
// at once with "no route to host", so apps fall back to IPv4 without waiting.
func (m *DarwinRouteManager) AddIPv6Reject() error {
	return m.addV6Halves(func(dst string) []string { return []string{"-inet6", "-reject", dst, "::1"} })
}

// AddIPv6SplitDefault routes all of IPv6 into the TUN (--enable_ipv6).
func (m *DarwinRouteManager) AddIPv6SplitDefault() error {
	return m.addV6Halves(func(dst string) []string { return []string{"-inet6", dst, "-interface", m.tunName} })
}

// addV6Halves falls back to `route change` on "File exists", so a reject route left by a
// crashed run does not block startup.
func (m *DarwinRouteManager) addV6Halves(spec func(dst string) []string) error {
	for _, dst := range []string{"::/1", "8000::/1"} {
		args := spec(dst)
		out, err := runCapture("route", append([]string{"-n", "add"}, args...)...)
		if err != nil && strings.Contains(out, "File exists") {
			m.warnForeignV6(dst)
			_, err = runCapture("route", append([]string{"-n", "change"}, args...)...)
		}
		if err != nil {
			return fmt.Errorf("IPv6 route %s not installed; IPv6 traffic would leak outside the tunnel: %w", strings.Join(args, " "), err)
		}
		m.addedV6 = append(m.addedV6, dst)
	}
	return nil
}

// warnForeignV6 names a route that `route change` is about to overwrite and that another
// tool, such as a second VPN, installed; its traffic moves into our reject or TUN route.
func (m *DarwinRouteManager) warnForeignV6(dst string) {
	out, err := runCaptureStdout("netstat", "-rn", "-f", "inet6")
	if err != nil {
		return
	}
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) < 4 || f[0] != dst {
			continue
		}
		gw, flags, netif := f[1], f[2], f[3]
		if strings.ContainsAny(flags, "RB") || netif == m.tunName {
			continue
		}
		logx.Warn("changing IPv6 route %s via %s on %s, which urnet-client did not install\n", dst, gw, netif)
	}
}

// Cleanup removes all routes applied by this manager and brings the TUN interface down.
func (m *DarwinRouteManager) Cleanup() {
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
	m.RemoveDNSBypass()
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
	// IPv6 reject/split-default routes
	for _, dst := range m.addedV6 {
		_ = runSudo("route", "-n", "delete", "-inet6", dst)
	}
	// Bring interface down
	_ = runSudo("ifconfig", m.tunName, "down")
	// Kill switch: keep blackhole in place (traffic stays blocked after VPN exits).
	// Without kill switch: remove blackhole and restore original default gateway.
	if m.killSwitchAdded {
		if m.killSwitch {
			logx.Info("kill switch: blackhole default route preserved; all traffic is blocked until you run: sudo route delete default && sudo route add default <gateway>\n")
		} else {
			_ = runSudo("route", "-n", "delete", "default")
			if m.defGw != "" {
				_ = runSudo("route", "-n", "add", "default", m.defGw)
			}
		}
	}
	// Logged independent of killSwitchAdded: the IPv6 blackhole is installed before the IPv4
	// one, so it can be up even when the IPv4 blackhole install then fails.
	if m.killSwitchV6Added {
		logx.Info("kill switch: IPv6 blackhole default preserved; to unblock IPv6 run: sudo route delete -inet6 default, then turn the network interface off and on\n")
	}
}

// addVariant tries multiple route command variants to install a split-default entry.
// The first variant that succeeds is recorded in addedSplits for cleanup.
func (m *DarwinRouteManager) addVariant(dest, mask string) bool {
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
		logx.Warn("failed to add split default for %s (%s)\n", dest, mask)
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

	logx.Warn("failed to add split default for %s (%s)\n", dest, mask)
	return false
}

// addScoped installs a split-default route scoped to the TUN interface (SOCKS-only mode).
func (m *DarwinRouteManager) addScoped(dest, mask string) {
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
