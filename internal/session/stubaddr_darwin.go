//go:build darwin

package session

import "net/netip"

// stubHost is loopback on macOS: mDNSResponder scopes queries for a stub on the TUN address
// to the TUN, so they would enter the tunnel instead of reaching the local stub, and the
// tunnel needs DNS to reach its providers.
func stubHost(netip.Addr) netip.Addr { return netip.AddrFrom4([4]byte{127, 0, 0, 1}) }
