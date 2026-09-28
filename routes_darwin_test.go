//go:build darwin

package main

import (
	"sync"
	"testing"
)

func TestDarwinKillSwitch_RestoresDefaultWhenBlackholeFails(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("route -n add -blackhole default", "route: writing to routing socket: Invalid argument")
	m := newDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	m.AddKillSwitchRoute()

	if m.killSwitchAdded {
		t.Fatal("killSwitchAdded must stay false when the blackhole route failed")
	}
	if f.count("route -n add default 192.168.1.1") != 1 {
		t.Fatalf("calls = %v; want the original default restored", f.Calls())
	}
}

func TestDarwinKillSwitch_NoRestoreWhenBlackholeInstalled(t *testing.T) {
	f := useFakeRunner(t)
	m := newDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")

	m.AddKillSwitchRoute()

	if !m.killSwitchAdded {
		t.Fatal("killSwitchAdded should be true")
	}
	if f.count("route -n add default 192.168.1.1") != 0 {
		t.Fatalf("calls = %v; default must not be re-added while the kill switch is active", f.Calls())
	}
}

func TestDarwinDNSBypass_RemoveAndCleanupDoNotRace(t *testing.T) {
	f := useFakeRunner(t)
	m := newDarwinRouteManager("utun9", "10.255.0.1", "192.168.1.1")
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
