//go:build darwin || linux

package netcfg

// tunIPv6CIDR is the TUN's IPv6 address; it is only assigned with --enable_ipv6.
const tunIPv6CIDR = "fd00::2/120"
