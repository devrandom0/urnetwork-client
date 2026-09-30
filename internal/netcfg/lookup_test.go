//go:build darwin || linux

package netcfg

import (
	"net"
	"testing"
)

func fakeLookup(t *testing.T, ips ...string) {
	t.Helper()
	old := lookupIP
	lookupIP = func(string) ([]net.IP, error) {
		var out []net.IP
		for _, ip := range ips {
			out = append(out, net.ParseIP(ip))
		}
		return out, nil
	}
	t.Cleanup(func() { lookupIP = old })
}
