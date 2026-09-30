package netcfg

import "net/netip"

// SystemDNS points the OS resolver at the DNS stub for one session and undoes it.
type SystemDNS interface {
	Apply(stub netip.Addr) error
	Restore() error
}
