//go:build linux

package main

import (
	"net"
	"strings"
)

// linuxRouteManager implements RouteManager for Linux using `ip route` commands.
// All additions are tracked so Cleanup can remove them precisely.
type linuxRouteManager struct {
	tunName string
	origGw  string // original default gateway IP
	origDev string // original default gateway device

	addedBypass  []string // host IPs routed via original gateway (bypass)
	addedExclude []string // exclude destinations routed via original gateway or unreachable
	addedExtra   []string // extra destinations routed through TUN
	addedDNSTun  []string // DNS server IPs routed through TUN
	addedSplits  bool     // whether 0.0.0.0/1 + 128.0.0.0/1 were installed
}

// newLinuxRouteManager creates a route manager for the named TUN interface.
// origGw and origDev are the pre-VPN default gateway IP and device (may be empty).
func newLinuxRouteManager(tunName, origGw, origDev string) *linuxRouteManager {
	return &linuxRouteManager{
		tunName: tunName,
		origGw:  origGw,
		origDev: origDev,
	}
}

func (m *linuxRouteManager) AddBypassEndpoint(rawURL string) {
	host := extractHost(rawURL)
	if host == "" || m.origDev == "" {
		return
	}
	ips, _ := net.LookupIP(host)
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		ipStr := v4.String()
		if m.origGw != "" {
			_ = run("ip", "route", "add", ipStr, "via", m.origGw, "dev", m.origDev)
		} else {
			_ = run("ip", "route", "add", ipStr, "dev", m.origDev)
		}
		m.addedBypass = append(m.addedBypass, ipStr)
	}
}

func (m *linuxRouteManager) AddSplitDefault() {
	_ = run("ip", "route", "add", "0.0.0.0/1", "dev", m.tunName)
	_ = run("ip", "route", "add", "128.0.0.0/1", "dev", m.tunName)
	m.addedSplits = true
}

func (m *linuxRouteManager) AddExclude(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	if m.origDev != "" && m.origGw != "" {
		_ = run("ip", "route", "add", dest, "via", m.origGw, "dev", m.origDev)
	} else if m.origDev != "" {
		_ = run("ip", "route", "add", dest, "dev", m.origDev)
	} else {
		_ = run("ip", "route", "add", dest, "unreachable")
	}
	m.addedExclude = append(m.addedExclude, dest)
}

func (m *linuxRouteManager) AddExtraRoute(dest string) {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return
	}
	_ = run("ip", "route", "add", dest, "dev", m.tunName)
	m.addedExtra = append(m.addedExtra, dest)
}

func (m *linuxRouteManager) AddDNSServerRoutes(ips []string, bypass bool) {
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if bypass {
			if m.origDev == "" {
				continue
			}
			if m.origGw != "" {
				_ = run("ip", "route", "add", ip, "via", m.origGw, "dev", m.origDev)
			} else {
				_ = run("ip", "route", "add", ip, "dev", m.origDev)
			}
			m.addedBypass = append(m.addedBypass, ip)
		} else {
			_ = run("ip", "route", "add", ip, "dev", m.tunName)
			m.addedDNSTun = append(m.addedDNSTun, ip)
		}
	}
}

// SetDNS is a no-op on Linux. DNS management on Linux is left to the caller.
func (m *linuxRouteManager) SetDNS(_ []string, _ string) error { return nil }

// Cleanup removes all routes added during this session and brings the TUN interface down.
func (m *linuxRouteManager) Cleanup() {
	if m.addedSplits {
		_ = run("ip", "route", "del", "0.0.0.0/1")
		_ = run("ip", "route", "del", "128.0.0.0/1")
	}
	for _, ip := range m.addedBypass {
		_ = run("ip", "route", "del", ip)
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
}
