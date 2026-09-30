//go:build linux

package netcfg

import (
	"fmt"
	"testing"
)

func TestLinuxBypassEndpoint_ReturnsHostAndIPv4(t *testing.T) {
	f := useFakeRunner(t)
	fakeLookup(t, "203.0.113.7", "2001:db8::7", "203.0.113.8")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	host, ips := m.AddBypassEndpoint("https://api.example.test:443/v1")

	if host != "api.example.test" || fmt.Sprint(ips) != "[203.0.113.7 203.0.113.8]" {
		t.Fatalf("got %q %v; want the host and its IPv4 addresses", host, ips)
	}
	for _, ip := range []string{"203.0.113.7", "203.0.113.8"} {
		if f.count("ip route add "+ip+" via 192.168.1.1 dev eth0") != 1 {
			t.Fatalf("calls = %v; want a bypass route for %s", f.Calls(), ip)
		}
	}
}

func TestLinuxBypassEndpoint_NoDeviceReturnsNothing(t *testing.T) {
	useFakeRunner(t)
	fakeLookup(t, "203.0.113.7")
	m := NewLinuxRouteManager("tun0", "", "")
	if host, ips := m.AddBypassEndpoint("https://api.example.test"); host != "" || ips != nil {
		t.Fatalf("got %q %v; nothing is routed around the tunnel, so nothing may be pinned", host, ips)
	}
}
