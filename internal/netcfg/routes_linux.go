//go:build linux

package netcfg

import (
	"fmt"
	"net/netip"
	"strings"
	"sync"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// LinuxRouteManager implements RouteManager for Linux using `ip route` commands.
// All additions are tracked so Cleanup can remove them precisely.
type LinuxRouteManager struct {
	tunName string
	origGw  string // original default gateway IP
	origDev string // original default gateway device

	addedBypass    []string   // control-plane host IPs routed via original gateway
	addedDNSBypass []string   // DNS server IPs routed via original gateway
	mu             sync.Mutex // guards addedDNSBypass; RemoveDNSBypass runs on the bootstrap goroutine
	addedExclude   []string   // exclude destinations routed via original gateway or unreachable
	addedExtra     []string   // extra destinations routed through TUN
	addedDNSTun    []string   // DNS server IPs routed through TUN
	addedSplits    []string   // split-default destinations installed through the TUN
	addedV6        [][]string // `ip -6 route del` arguments for each IPv6 route we own

	killSwitch        bool // whether kill-switch mode is active
	killSwitchAdded   bool // whether the blackhole default was successfully installed
	killSwitchV6Added bool // whether the IPv6 blackhole default was installed
}

// NewLinuxRouteManager creates a route manager for the named TUN interface.
// origGw and origDev are the pre-VPN default gateway IP and device (may be empty).
func NewLinuxRouteManager(tunName, origGw, origDev string) *LinuxRouteManager {
	return &LinuxRouteManager{
		tunName: tunName,
		origGw:  origGw,
		origDev: origDev,
	}
}

// addRoute reports whether this session now owns the route. Any failure, including
// "File exists", means the route is not ours and must stay out of Cleanup.
func (m *LinuxRouteManager) addRoute(args ...string) bool {
	if err := run("ip", append([]string{"route", "add"}, args...)...); err != nil {
		logx.Warn("ip route add %s failed: %v\n", strings.Join(args, " "), err)
		return false
	}
	return true
}

// AddBypassEndpoint routes the IPv4 addresses of rawURL's host around the tunnel and
// returns the host and those addresses, so the DNS stub can answer for it without the tunnel.
func (m *LinuxRouteManager) AddBypassEndpoint(rawURL string) (string, []netip.Addr) {
	host := extractHost(rawURL)
	if host == "" || m.origDev == "" {
		return "", nil
	}
	ips, _ := lookupIP(host)
	var routed []netip.Addr
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		addr, _ := netip.AddrFromSlice(v4)
		routed = append(routed, addr)
		ipStr := v4.String()
		var ok bool
		if m.origGw != "" {
			ok = m.addRoute(ipStr, "via", m.origGw, "dev", m.origDev)
		} else {
			ok = m.addRoute(ipStr, "dev", m.origDev)
		}
		if ok {
			m.addedBypass = append(m.addedBypass, ipStr)
		}
	}
	return host, routed
}

func (m *LinuxRouteManager) AddSplitDefault() error {
	var failed []string
	for _, dst := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		if m.addRoute(dst, "dev", m.tunName) {
			m.addedSplits = append(m.addedSplits, dst)
		} else {
			failed = append(failed, dst)
		}
	}
	if len(failed) > 0 {
		var hints []string
		for _, dst := range failed {
			hints = append(hints, fmt.Sprintf("sudo ip route del %s", dst))
		}
		return fmt.Errorf("split default route %s via %s not installed; traffic would leak outside the tunnel; if a stale route exists, run: %s",
			strings.Join(failed, ", "), m.tunName, strings.Join(hints, " && "))
	}
	return nil
}

// AddKillSwitchRoute installs a blackhole default route so that if the VPN split
// routes are removed, all traffic is blocked rather than leaking via the real
// default gateway. Call this before AddSplitDefault so the /1 routes take priority.
// The route is left in place on Cleanup when kill-switch mode is active.
// On failure the original default is restored and an error is returned so startup aborts.
func (m *LinuxRouteManager) AddKillSwitchRoute() error {
	m.killSwitch = true
	if err := m.addKillSwitchV6(); err != nil {
		return err
	}
	var restore []string
	switch {
	case m.origGw != "" && m.origDev != "":
		if run("ip", "route", "del", "default", "via", m.origGw, "dev", m.origDev) == nil {
			restore = []string{"ip", "route", "add", "default", "via", m.origGw, "dev", m.origDev}
		}
	case m.origDev != "":
		if run("ip", "route", "del", "default", "dev", m.origDev) == nil {
			restore = []string{"ip", "route", "add", "default", "dev", m.origDev}
		}
	default:
		_ = run("ip", "route", "del", "default")
	}
	err := run("ip", "route", "add", "blackhole", "default")
	if err == nil {
		m.killSwitchAdded = true
		logx.Info("kill switch: blackhole default route installed\n")
		return nil
	}
	if restore == nil {
		return fmt.Errorf("kill switch: install blackhole default route: %w", err)
	}
	manual := strings.Join(restore, " ")
	if rErr := run(restore[0], restore[1:]...); rErr != nil {
		logx.Error("kill switch: could not restore default route: %v; run: %s\n", rErr, manual)
		return fmt.Errorf("kill switch: install blackhole default route: %w; restoring the default route also failed, run: %s", err, manual)
	}
	return fmt.Errorf("kill switch: install blackhole default route: %w; original default route restored", err)
}

// addKillSwitchV6 installs an IPv6 blackhole default with metric 1 instead of deleting the
// IPv6 default: router advertisements would reinstall a deleted default within minutes.
func (m *LinuxRouteManager) addKillSwitchV6() error {
	if !IPv6Available() {
		return nil
	}
	if err := run("ip", "-6", "route", "replace", "blackhole", "default", "metric", "1"); err != nil {
		return fmt.Errorf("kill switch: install IPv6 blackhole default route: %w", err)
	}
	m.killSwitchV6Added = true
	return nil
}

// AddIPv6Reject makes every IPv6 destination outside connected and link-local prefixes fail
// at once with "no route to host", so apps fall back to IPv4 without waiting.
func (m *LinuxRouteManager) AddIPv6Reject() error {
	return m.addV6Halves("unreachable")
}

// AddIPv6SplitDefault routes all of IPv6 into the TUN (--enable_ipv6).
func (m *LinuxRouteManager) AddIPv6SplitDefault() error {
	return m.addV6Halves("", "dev", m.tunName)
}

// addV6Halves uses replace so a reject route left by a crashed run does not block startup.
func (m *LinuxRouteManager) addV6Halves(routeType string, via ...string) error {
	for _, dst := range []string{"::/1", "8000::/1"} {
		route := []string{dst}
		if routeType != "" {
			route = []string{routeType, dst}
		}
		route = append(route, via...)
		if err := run("ip", append([]string{"-6", "route", "replace"}, route...)...); err != nil {
			return fmt.Errorf("IPv6 route %s not installed; IPv6 traffic would leak outside the tunnel: %w", strings.Join(route, " "), err)
		}
		m.addedV6 = append(m.addedV6, route)
	}
	return nil
}

func (m *LinuxRouteManager) AddExclude(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	var ok bool
	switch {
	case m.origDev != "" && m.origGw != "":
		ok = m.addRoute(dest, "via", m.origGw, "dev", m.origDev)
	case m.origDev != "":
		ok = m.addRoute(dest, "dev", m.origDev)
	default:
		ok = m.addRoute("unreachable", dest)
	}
	if ok {
		m.addedExclude = append(m.addedExclude, dest)
	}
}

func (m *LinuxRouteManager) AddExtraRoute(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	if m.addRoute(dest, "dev", m.tunName) {
		m.addedExtra = append(m.addedExtra, dest)
	}
}

func (m *LinuxRouteManager) AddDNSServerRoutes(ips []string, bypass bool) {
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if !bypass {
			if m.addRoute(ip, "dev", m.tunName) {
				m.addedDNSTun = append(m.addedDNSTun, ip)
			}
			continue
		}
		if m.origDev == "" {
			continue
		}
		var ok bool
		if m.origGw != "" {
			ok = m.addRoute(ip, "via", m.origGw, "dev", m.origDev)
		} else {
			ok = m.addRoute(ip, "dev", m.origDev)
		}
		if ok {
			m.mu.Lock()
			m.addedDNSBypass = append(m.addedDNSBypass, ip)
			m.mu.Unlock()
		}
	}
}

// RemoveDNSBypass deletes the DNS bypass routes. The dns_bootstrap=cache goroutine and
// Cleanup may both call it; the mutex makes each route go exactly once.
func (m *LinuxRouteManager) RemoveDNSBypass() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ip := range m.addedDNSBypass {
		_ = run("ip", "route", "del", ip)
	}
	m.addedDNSBypass = nil
}

// SetDNS is a no-op on Linux. DNS management on Linux is left to the caller.
func (m *LinuxRouteManager) SetDNS(_ []string, _ string) error { return nil }

// Cleanup removes all routes added during this session and brings the TUN interface down.
func (m *LinuxRouteManager) Cleanup() {
	for _, dst := range m.addedSplits {
		_ = run("ip", "route", "del", dst)
	}
	for _, ip := range m.addedBypass {
		_ = run("ip", "route", "del", ip)
	}
	m.RemoveDNSBypass()
	for _, r := range m.addedV6 {
		_ = run("ip", append([]string{"-6", "route", "del"}, r...)...)
	}
	for _, r := range m.addedExclude {
		_ = run("ip", "route", "del", r)
	}
	for _, r := range m.addedExtra {
		_ = run("ip", "route", "del", r)
	}
	for _, d := range m.addedDNSTun {
		_ = run("ip", "route", "del", d)
	}
	_ = run("ip", "link", "set", m.tunName, "down")
	_ = run("ip", "addr", "flush", "dev", m.tunName)
	// Kill switch: keep blackhole in place (traffic stays blocked after VPN exits).
	// Without kill switch: remove blackhole and restore original default gateway.
	if m.killSwitchAdded {
		if m.killSwitch {
			logx.Info("kill switch: blackhole default route preserved; all traffic is blocked until you run: ip route del blackhole default && ip route add default via <gateway>\n")
			if m.killSwitchV6Added {
				logx.Info("kill switch: IPv6 blackhole default preserved; to unblock IPv6 run: ip -6 route del blackhole default metric 1\n")
			}
		} else {
			_ = run("ip", "route", "del", "blackhole", "default")
			if m.origGw != "" && m.origDev != "" {
				_ = run("ip", "route", "add", "default", "via", m.origGw, "dev", m.origDev)
			}
		}
	}
}
