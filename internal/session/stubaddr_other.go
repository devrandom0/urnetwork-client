//go:build !darwin

package session

import "net/netip"

// stubHost is the TUN address, which the kernel delivers locally like any own address.
func stubHost(tun netip.Addr) netip.Addr { return tun }
