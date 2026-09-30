//go:build linux

package netcfg

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
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

func TestLinuxIPv6Reject_InstallsUnreachableHalvesAndCleansUp(t *testing.T) {
	f := useFakeRunner(t)
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddIPv6Reject(); err != nil {
		t.Fatalf("AddIPv6Reject: %v", err)
	}
	m.Cleanup()

	for _, dst := range []string{"::/1", "8000::/1"} {
		if f.count("ip -6 route replace unreachable "+dst) != 1 || f.count("ip -6 route del unreachable "+dst) != 1 {
			t.Fatalf("calls = %v; want %s installed once and removed once", f.Calls(), dst)
		}
	}
}

func TestLinuxIPv6Reject_FailureIsErrorAndNotCleanedUp(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip -6 route replace unreachable 8000::/1", "RTNETLINK answers: Operation not permitted")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	err := m.AddIPv6Reject()

	if err == nil || !strings.Contains(err.Error(), "leak") {
		t.Fatalf("AddIPv6Reject = %v; want an error so startup aborts", err)
	}
	m.Cleanup()
	if f.count("ip -6 route del unreachable 8000::/1") != 0 || f.count("ip -6 route del unreachable ::/1") != 1 {
		t.Fatalf("calls = %v; only the installed half may be removed", f.Calls())
	}
}

func TestLinuxIPv6SplitDefault_RoutesHalvesIntoTUN(t *testing.T) {
	f := useFakeRunner(t)
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddIPv6SplitDefault(); err != nil {
		t.Fatalf("AddIPv6SplitDefault: %v", err)
	}
	m.Cleanup()

	for _, dst := range []string{"::/1", "8000::/1"} {
		if f.count("ip -6 route replace "+dst+" dev tun0") != 1 || f.count("ip -6 route del "+dst+" dev tun0") != 1 {
			t.Fatalf("calls = %v; want %s routed into tun0 and removed on cleanup", f.Calls(), dst)
		}
	}
}

func TestLinuxKillSwitch_InstallsIPv6BlackholeFirstAndKeepsIt(t *testing.T) {
	f := useFakeRunner(t)
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddKillSwitchRoute(); err != nil {
		t.Fatalf("AddKillSwitchRoute: %v", err)
	}
	m.Cleanup()

	calls := f.Calls()
	if len(calls) == 0 || calls[0] != "ip -6 route replace blackhole default metric 1" {
		t.Fatalf("calls = %v; want the IPv6 blackhole before any IPv4 change", calls)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "ip -6 route del blackhole") {
			t.Fatalf("Cleanup ran %q; the kill switch keeps IPv6 blocked after exit", c)
		}
	}
}

func TestLinuxKillSwitch_IPv6FailureAbortsBeforeIPv4(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip -6 route replace blackhole default metric 1", "RTNETLINK answers: Operation not permitted")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddKillSwitchRoute(); err == nil {
		t.Fatal("AddKillSwitchRoute = nil; want an error")
	}
	for _, c := range f.Calls() {
		if strings.HasPrefix(c, "ip route") {
			t.Fatalf("ran %q after the IPv6 blackhole failed; the IPv4 default must stay untouched", c)
		}
	}
}

func TestLinuxKillSwitch_ContinuesWhenKernelRefusesIPv6(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip -6 route replace blackhole default metric 1", "RTNETLINK answers: Address family not supported by protocol")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddKillSwitchRoute(); err != nil {
		t.Fatalf("AddKillSwitchRoute = %v; with IPv6 off in the kernel there is no IPv6 to block", err)
	}
	if m.killSwitchV6Added || f.count("ip route add blackhole default") != 1 {
		t.Fatalf("calls = %v v6Added=%v; want only the IPv4 blackhole", f.Calls(), m.killSwitchV6Added)
	}
}

func TestLinuxIPv6Reject_AlwaysAttemptedAndBenignWhenKernelRefusesIPv6(t *testing.T) {
	for _, out := range []string{
		"RTNETLINK answers: Address family not supported by protocol",
		"RTNETLINK answers: Permission denied",
	} {
		t.Run(out, func(t *testing.T) {
			f := useFakeRunner(t)
			f.failOn("ip -6 route replace unreachable ::/1", out)
			f.failOn("ip -6 route replace unreachable 8000::/1", out)
			m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

			if err := m.AddIPv6Reject(); err != nil {
				t.Fatalf("AddIPv6Reject = %v; the kernel has IPv6 off, so nothing can leak", err)
			}
			m.Cleanup()
			if f.count("ip -6 route del unreachable ::/1") != 0 {
				t.Fatalf("calls = %v; a route never installed must not be deleted", f.Calls())
			}
		})
	}
}

func TestLinuxIPv6Halves_WarnsWhenReplacingForeignRoute(t *testing.T) {
	f := useFakeRunner(t)
	f.results["ip -6 route show exact ::/1"] = fakeResult{stdout: "::/1 dev wg0 metric 1024 pref medium\n"}
	f.results["ip -6 route show exact 8000::/1"] = fakeResult{stdout: "unreachable 8000::/1 dev lo metric 1024 pref medium\n"}
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	logged := captureStderr(t, func() {
		if err := m.AddIPv6Reject(); err != nil {
			t.Error(err)
		}
	})

	if !strings.Contains(logged, "::/1 dev wg0") {
		t.Fatalf("stderr = %q; want a WARN naming the foreign route being replaced", logged)
	}
	if strings.Contains(logged, "unreachable 8000::/1") {
		t.Fatalf("stderr = %q; a leftover reject route is ours to take over quietly", logged)
	}
}

func TestLinuxDNSBypass_RemoveAndCleanupDoNotRace(t *testing.T) {
	f := useFakeRunner(t)
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	m.AddDNSServerRoutes([]string{"1.1.1.1", "9.9.9.9"}, true)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m.RemoveDNSBypass() }()
	go func() { defer wg.Done(); m.Cleanup() }()
	wg.Wait()

	for _, ip := range []string{"1.1.1.1", "9.9.9.9"} {
		if n := f.count("ip route del " + ip); n != 1 {
			t.Fatalf("%s deleted %d times, want exactly 1", ip, n)
		}
	}
}

func TestLinuxDNSBypass_RemoveKeepsControlPlaneRoutes(t *testing.T) {
	f := useFakeRunner(t)
	fakeLookup(t, "203.0.113.7")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	m.AddBypassEndpoint("https://api.example.test")
	m.AddDNSServerRoutes([]string{"1.1.1.1"}, true)

	m.RemoveDNSBypass()

	if f.count("ip route del 1.1.1.1") != 1 || f.count("ip route del 203.0.113.7") != 0 {
		t.Fatalf("calls = %v; RemoveDNSBypass must drop only DNS routes", f.Calls())
	}
}

func TestLinuxKillSwitch_IPv6LogSurvivesIPv4Failure(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip route add blackhole default", "RTNETLINK answers: Operation not permitted")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddKillSwitchRoute(); err == nil {
		t.Fatal("AddKillSwitchRoute = nil; want an error when the IPv4 blackhole fails")
	}
	if !m.killSwitchV6Added {
		t.Fatal("killSwitchV6Added must stay true; the IPv6 blackhole was installed before the IPv4 failure")
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	m.Cleanup()
	_ = w.Close()
	os.Stdout = origStdout
	out, _ := io.ReadAll(r)

	if !strings.Contains(string(out), "IPv6 blackhole default preserved") {
		t.Fatalf("Cleanup log = %q; want the IPv6 hint logged even though the IPv4 blackhole never installed", out)
	}
}
