//go:build darwin

package main

import "testing"

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
