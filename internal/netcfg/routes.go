package netcfg

import "net/netip"

// RouteManager handles OS-level route mutations for a VPN session.
// All Add* methods record what they changed; Cleanup undoes all of it.
// Call Cleanup via defer immediately after creating the manager.
type RouteManager interface {
	// AddBypassEndpoint resolves rawURL (e.g. "https://api.example.com") and
	// installs host routes for its IPv4 addresses via the pre-VPN default gateway,
	// so control-plane traffic bypasses the TUN. It returns the host and those addresses.
	AddBypassEndpoint(rawURL string) (string, []netip.Addr)

	// AddSplitDefault installs split-default routes (0.0.0.0/1 and 128.0.0.0/1)
	// through the managed TUN, making it the effective default gateway.
	// It returns an error if either half is missing so startup can abort before traffic leaks.
	AddSplitDefault() error

	// AddIPv6Reject installs reject routes for ::/1 and 8000::/1 (IPv6 blocked).
	AddIPv6Reject() error

	// AddIPv6SplitDefault routes ::/1 and 8000::/1 through the TUN (--enable_ipv6).
	AddIPv6SplitDefault() error

	// AddKillSwitchRoute installs blackhole defaults for IPv6 and IPv4 that stay after exit.
	AddKillSwitchRoute() error

	// AddExclude installs a route for dest (host or CIDR) that bypasses the TUN.
	// Routes it via the original gateway or a reject route when no gateway is available.
	AddExclude(dest string)

	// AddExtraRoute installs an explicit route for dest (host or CIDR) through the TUN.
	AddExtraRoute(dest string)

	// AddDNSServerRoutes installs routes for the given DNS server IPs.
	// bypass=true routes them via the original gateway; bypass=false routes them through the TUN.
	AddDNSServerRoutes(ips []string, bypass bool)

	// RemoveDNSBypass drops the bypass=true DNS routes early (dns_bootstrap=cache).
	RemoveDNSBypass()

	// Cleanup removes all routes applied by this manager.
	Cleanup()
}
