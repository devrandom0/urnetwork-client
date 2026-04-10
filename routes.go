package main

// RouteManager handles OS-level route mutations for a VPN session.
// All Add* and SetDNS methods record what they changed; Cleanup undoes all of it.
// Call Cleanup via defer immediately after creating the manager.
type RouteManager interface {
	// AddBypassEndpoint resolves rawURL (e.g. "https://api.example.com") and
	// installs host routes for its IPv4 addresses via the pre-VPN default gateway,
	// so control-plane traffic bypasses the TUN.
	AddBypassEndpoint(rawURL string)

	// AddSplitDefault installs split-default routes (0.0.0.0/1 and 128.0.0.0/1)
	// through the managed TUN, making it the effective default gateway.
	AddSplitDefault()

	// AddExclude installs a route for dest (host or CIDR) that bypasses the TUN.
	// Routes it via the original gateway or a reject route when no gateway is available.
	AddExclude(dest string)

	// AddExtraRoute installs an explicit route for dest (host or CIDR) through the TUN.
	AddExtraRoute(dest string)

	// AddDNSServerRoutes installs routes for the given DNS server IPs.
	// bypass=true routes them via the original gateway (used when --default_route is set).
	// bypass=false routes them through the TUN.
	AddDNSServerRoutes(ips []string, bypass bool)

	// SetDNS configures system-wide DNS resolvers.
	// On macOS this calls networksetup; on Linux this is a no-op and returns nil.
	SetDNS(servers []string, service string) error

	// Cleanup removes all routes and DNS configuration applied by this manager.
	Cleanup()
}
