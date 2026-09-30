//go:build darwin

package netcfg

import (
	"fmt"
	"io"
	"os"
	"strings"
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

func TestDarwinIPv6Reject_InstallsRejectHalvesAndCleansUp(t *testing.T) {
	f := useFakeRunner(t)
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddIPv6Reject(); err != nil {
		t.Fatalf("AddIPv6Reject: %v", err)
	}
	m.Cleanup()

	for _, dst := range []string{"::/1", "8000::/1"} {
		if f.count("route -n add -inet6 -reject "+dst+" ::1") != 1 || f.count("route -n delete -inet6 "+dst) != 1 {
			t.Fatalf("calls = %v; want %s installed once and removed once", f.Calls(), dst)
		}
	}
}

func TestDarwinIPv6Reject_TakesOverLeftoverRoute(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -inet6 -reject ::/1 ::1", "route: writing to routing socket: File exists")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddIPv6Reject(); err != nil {
		t.Fatalf("AddIPv6Reject = %v; a reject route left by a crash must not block startup", err)
	}
	if f.count("route -n change -inet6 -reject ::/1 ::1") != 1 {
		t.Fatalf("calls = %v; want the existing route changed", f.Calls())
	}
}

func TestDarwinIPv6Reject_FailureIsError(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -inet6 -reject 8000::/1 ::1", "route: writing to routing socket: Invalid argument")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddIPv6Reject(); err == nil || !strings.Contains(err.Error(), "leak") {
		t.Fatalf("AddIPv6Reject = %v; want an error so startup aborts", err)
	}
	m.Cleanup()
	if f.count("route -n delete -inet6 8000::/1") != 0 {
		t.Fatalf("calls = %v; a route that was never installed must not be deleted", f.Calls())
	}
}

func TestDarwinIPv6SplitDefault_RoutesHalvesIntoTUN(t *testing.T) {
	f := useFakeRunner(t)
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddIPv6SplitDefault(); err != nil {
		t.Fatalf("AddIPv6SplitDefault: %v", err)
	}
	m.Cleanup()

	for _, dst := range []string{"::/1", "8000::/1"} {
		if f.count("route -n add -inet6 "+dst+" -interface utun9") != 1 || f.count("route -n delete -inet6 "+dst) != 1 {
			t.Fatalf("calls = %v; want %s routed into utun9 and removed on cleanup", f.Calls(), dst)
		}
	}
}

func TestDarwinKillSwitch_InstallsIPv6BlackholeFirstAndKeepsIt(t *testing.T) {
	f := useFakeRunner(t)
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddKillSwitchRoute(); err != nil {
		t.Fatalf("AddKillSwitchRoute: %v", err)
	}
	m.Cleanup()

	calls := f.Calls()
	if len(calls) == 0 || calls[0] != "route -n add -inet6 -blackhole default ::1" {
		t.Fatalf("calls = %v; want the IPv6 blackhole before any IPv4 change", calls)
	}
	if f.count("route -n delete -inet6 default") != 0 {
		t.Fatalf("calls = %v; the kill switch keeps IPv6 blocked after exit", calls)
	}
}

func TestDarwinKillSwitch_IPv6FileExistsReplacesDefault(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -inet6 -blackhole default ::1", "route: writing to routing socket: File exists")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	_ = m.AddKillSwitchRoute()

	if f.count("route -n delete -inet6 default") != 1 || f.count("route -n add -inet6 -blackhole default ::1") != 2 {
		t.Fatalf("calls = %v; want the router default deleted and the blackhole retried", f.Calls())
	}
}

func TestDarwinKillSwitch_IPv6FailureAbortsBeforeIPv4(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -inet6 -blackhole default ::1", "route: writing to routing socket: Network is unreachable")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddKillSwitchRoute(); err == nil {
		t.Fatal("AddKillSwitchRoute = nil; want an error")
	}
	if f.count("route -n delete default") != 0 {
		t.Fatalf("calls = %v; the IPv4 default must stay untouched after the IPv6 step failed", f.Calls())
	}
}

func TestDarwinKillSwitch_IPv6LogSurvivesIPv4Failure(t *testing.T) {
	useFakeRunner(t).failOn("route -n add -blackhole default", "route: writing to routing socket: Invalid argument")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddKillSwitchRoute(); err == nil {
		t.Fatal("AddKillSwitchRoute = nil; want an error when the IPv4 blackhole fails")
	}
	if !m.killSwitchV6Added {
		t.Fatal("killSwitchV6Added must stay true; the IPv6 blackhole was installed before the IPv4 failure")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = origStdout
		_ = r.Close()
	})

	m.Cleanup()
	_ = w.Close()
	out, _ := io.ReadAll(r)

	if !strings.Contains(string(out), "IPv6 blackhole default preserved") {
		t.Fatalf("Cleanup log = %q; want the IPv6 hint logged even though the IPv4 blackhole never installed", out)
	}
}

const darwinV6Table = `Routing tables

Internet6:
Destination                             Gateway                                 Flags               Netif Expire
default                                 fe80::1%en0                             UGcg                  en0
::/1                                    fe80::99%en0                            UGSc                  en0
::1                                     ::1                                     UHL                   lo0
8000::/1                                ::1                                     UGRSc                 lo0
`

func TestDarwinIPv6Halves_WarnsWhenChangingForeignRoute(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -inet6 -reject ::/1 ::1", "route: writing to routing socket: File exists")
	f.failOn("route -n add -inet6 -reject 8000::/1 ::1", "route: writing to routing socket: File exists")
	f.results["netstat -rn -f inet6"] = fakeResult{stdout: darwinV6Table}
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	logged := captureStderr(t, func() {
		if err := m.AddIPv6Reject(); err != nil {
			t.Error(err)
		}
	})

	if !strings.Contains(logged, "::/1 via fe80::99%en0 on en0") {
		t.Fatalf("stderr = %q; want a WARN naming the foreign route being changed", logged)
	}
	if strings.Contains(logged, "8000::/1") {
		t.Fatalf("stderr = %q; a leftover reject route is ours to take over quietly", logged)
	}
}
