//go:build darwin

package netcfg

import (
	"fmt"
	"testing"
)

func TestDarwinBypassEndpoint_ReturnsHostAndIPv4(t *testing.T) {
	f := useFakeRunner(t)
	fakeLookup(t, "203.0.113.7", "2001:db8::7", "203.0.113.8")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	host, ips := m.AddBypassEndpoint("https://api.example.test:443/v1")

	if host != "api.example.test" || fmt.Sprint(ips) != "[203.0.113.7 203.0.113.8]" {
		t.Fatalf("got %q %v; want the host and its IPv4 addresses", host, ips)
	}
	for _, ip := range []string{"203.0.113.7", "203.0.113.8"} {
		if f.count("route -n add -host "+ip+" 192.168.1.1") != 1 {
			t.Fatalf("calls = %v; want a bypass route for %s", f.Calls(), ip)
		}
	}
}

func TestDarwinBypassEndpoint_NoGatewayReturnsNothing(t *testing.T) {
	useFakeRunner(t)
	fakeLookup(t, "203.0.113.7")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "")
	if host, ips := m.AddBypassEndpoint("https://api.example.test"); host != "" || ips != nil {
		t.Fatalf("got %q %v; nothing is routed around the tunnel, so nothing may be pinned", host, ips)
	}
}
