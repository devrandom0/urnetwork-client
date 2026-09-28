//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxRoutes_RecordOnlyOnSuccess(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip route add 128.0.0.0/1 dev tun0", "RTNETLINK answers: File exists")
	f.failOn("ip route add 10.0.0.0/8 via 192.168.1.1 dev eth0", "RTNETLINK answers: File exists")
	f.failOn("ip route add 172.16.0.0/12 dev tun0", "RTNETLINK answers: File exists")
	f.failOn("ip route add 1.1.1.1 dev tun0", "RTNETLINK answers: File exists")

	m := newLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	m.AddSplitDefault()
	m.AddExclude("10.0.0.0/8")
	m.AddExclude("10.1.0.0/16")
	m.AddExtraRoute("172.16.0.0/12")
	m.AddExtraRoute("172.20.0.0/16")
	m.AddDNSServerRoutes([]string{"1.1.1.1", "8.8.8.8"}, false)
	m.AddBypassEndpoint("https://203.0.113.7")
	m.Cleanup()

	for _, owned := range []string{"0.0.0.0/1", "10.1.0.0/16", "172.20.0.0/16", "8.8.8.8", "203.0.113.7"} {
		if f.count("ip route del "+owned) != 1 {
			t.Errorf("route %s was added by us and must be deleted exactly once", owned)
		}
	}
	for _, foreign := range []string{"128.0.0.0/1", "10.0.0.0/8", "172.16.0.0/12", "1.1.1.1"} {
		if f.count("ip route del "+foreign) != 0 {
			t.Errorf("route %s already existed and must not be deleted by Cleanup", foreign)
		}
	}
}

func TestLinuxRoutes_CleanupDeletesEveryAddedRoute(t *testing.T) {
	f := useFakeRunner(t)
	m := newLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	m.AddSplitDefault()
	m.AddExclude("10.1.0.0/16")
	m.AddExtraRoute("172.20.0.0/16")
	m.AddDNSServerRoutes([]string{"9.9.9.9"}, true)
	m.Cleanup()

	for _, c := range f.Calls() {
		fields := strings.Fields(c)
		if len(fields) < 4 || fields[0] != "ip" || fields[1] != "route" || fields[2] != "add" {
			continue
		}
		dest := fields[3]
		if f.count("ip route del "+dest) != 1 {
			t.Errorf("added %q but Cleanup did not delete %s exactly once", c, dest)
		}
	}
}

func TestLinuxRoutes_ExcludeWithoutGatewayUsesUnreachableType(t *testing.T) {
	f := useFakeRunner(t)
	m := newLinuxRouteManager("tun0", "", "")
	m.AddExclude("10.9.0.0/16")
	if f.count("ip route add unreachable 10.9.0.0/16") != 1 {
		t.Fatalf("calls = %v; want `ip route add unreachable 10.9.0.0/16`", f.Calls())
	}
}
