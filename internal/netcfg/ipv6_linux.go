//go:build linux

package netcfg

import "strings"

// ipv6Refused reports an `ip -6` failure that means the kernel has IPv6 off, so there is
// nothing to leak: EAFNOSUPPORT with ipv6.disable=1, EACCES with disable_ipv6 set. EPERM
// (missing CAP_NET_ADMIN) and every other failure stay errors.
func ipv6Refused(out string) bool {
	return strings.Contains(out, "Address family not supported") || strings.Contains(out, "Permission denied")
}

// v6RouteIsOurs reports whether an `ip -6 route show exact` line is a reject route or one
// through our TUN, which a crashed run may have left and we may take over quietly.
func v6RouteIsOurs(route, tun string) bool {
	f := strings.Fields(route)
	if len(f) == 0 {
		return true
	}
	switch f[0] {
	case "unreachable", "prohibit", "blackhole":
		return true
	}
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "dev" && f[i+1] == tun {
			return true
		}
	}
	return false
}
