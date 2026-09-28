//go:build darwin

package netcfg

import (
	"strings"
	"sync"
	"testing"
)

func TestDarwinKillSwitch_RestoresDefaultWhenBlackholeFails(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -blackhole default", "route: writing to routing socket: Invalid argument")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	_ = m.AddKillSwitchRoute()

	if m.killSwitchAdded {
		t.Fatal("killSwitchAdded must stay false when the blackhole route failed")
	}
	if f.count("route -n add default 192.168.1.1") != 1 {
		t.Fatalf("calls = %v; want the original default restored", f.Calls())
	}
}

func TestDarwinKillSwitch_NoRestoreWhenBlackholeInstalled(t *testing.T) {
	f := useFakeRunner(t)
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	_ = m.AddKillSwitchRoute()

	if !m.killSwitchAdded {
		t.Fatal("killSwitchAdded should be true")
	}
	if f.count("route -n add default 192.168.1.1") != 0 {
		t.Fatalf("calls = %v; default must not be re-added while the kill switch is active", f.Calls())
	}
}

func TestDarwinDNSBypass_RemoveAndCleanupDoNotRace(t *testing.T) {
	f := useFakeRunner(t)
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")
	m.AddDNSServerRoutes([]string{"1.1.1.1", "8.8.8.8"}, true)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m.RemoveDNSBypass() }()
	go func() { defer wg.Done(); m.Cleanup() }()
	wg.Wait()

	for _, ip := range []string{"1.1.1.1", "8.8.8.8"} {
		if n := f.count("route -n delete -host " + ip); n != 1 {
			t.Fatalf("%s deleted %d times, want exactly 1", ip, n)
		}
	}
}

func TestDarwinKillSwitch_BlackholeFailureReturnsError(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -blackhole default", "route: writing to routing socket: Invalid argument")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddKillSwitchRoute(); err == nil {
		t.Fatal("AddKillSwitchRoute = nil; want an error so startup aborts without leak protection")
	}
}

func TestDarwinKillSwitch_RestoreFailureNamesManualCommand(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -blackhole default", "route: writing to routing socket: Invalid argument")
	f.failOn("route -n add default 192.168.1.1", "route: writing to routing socket: Network is unreachable")
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	err := m.AddKillSwitchRoute()
	if err == nil || !strings.Contains(err.Error(), "sudo route add default 192.168.1.1") {
		t.Fatalf("AddKillSwitchRoute = %v; want an error naming the manual restore command", err)
	}
}

func TestDarwinSplitDefault_ErrorsWhenAHalfFails(t *testing.T) {
	f := useFakeRunner(t)
	for _, line := range []string{
		"route -n add -net 128.0.0.0 -netmask 128.0.0.0 -interface utun9",
		"route -n add -net 128.0.0.0/1 -interface utun9",
		"route -n add -net 128.0.0.0 -netmask 128.0.0.0 10.255.0.1 -ifscope utun9",
		"route -n add -net 128.0.0.0/1 10.255.0.1 -ifscope utun9",
	} {
		f.failOn(line, "route: writing to routing socket: Invalid argument")
	}
	m := NewDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	if err := m.AddSplitDefault(); err == nil {
		t.Fatal("AddSplitDefault = nil with 128.0.0.0/1 failing; want an error")
	}
}
