//go:build linux

package netcfg

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

	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	_ = m.AddSplitDefault()
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
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	_ = m.AddSplitDefault()
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
	m := NewLinuxRouteManager("tun0", "", "")
	m.AddExclude("10.9.0.0/16")
	if f.count("ip route add unreachable 10.9.0.0/16") != 1 {
		t.Fatalf("calls = %v; want `ip route add unreachable 10.9.0.0/16`", f.Calls())
	}
}

func TestLinuxKillSwitch_InstalledReturnsNil(t *testing.T) {
	f := useFakeRunner(t)
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddKillSwitchRoute(); err != nil {
		t.Fatalf("AddKillSwitchRoute = %v; want nil when the blackhole is installed", err)
	}
	if !m.killSwitchAdded {
		t.Fatal("killSwitchAdded should be true")
	}
	if f.count("ip route add default via 192.168.1.1 dev eth0") != 0 {
		t.Fatalf("calls = %v; default must not be re-added while the kill switch is active", f.Calls())
	}
}

func TestLinuxKillSwitch_BlackholeFailureRestoresDefaultAndErrors(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip route add blackhole default", "RTNETLINK answers: Operation not permitted")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddKillSwitchRoute(); err == nil {
		t.Fatal("AddKillSwitchRoute = nil; want an error so startup aborts without leak protection")
	}
	if m.killSwitchAdded {
		t.Fatal("killSwitchAdded must stay false when the blackhole route failed")
	}
	if f.count("ip route add default via 192.168.1.1 dev eth0") != 1 {
		t.Fatalf("calls = %v; want the original default restored", f.Calls())
	}
}

func TestLinuxKillSwitch_RestoreFailureNamesManualCommand(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip route add blackhole default", "RTNETLINK answers: Operation not permitted")
	f.failOn("ip route add default via 192.168.1.1 dev eth0", "RTNETLINK answers: Network is unreachable")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	err := m.AddKillSwitchRoute()
	if err == nil || !strings.Contains(err.Error(), "ip route add default via 192.168.1.1 dev eth0") {
		t.Fatalf("AddKillSwitchRoute = %v; want an error naming the manual restore command", err)
	}
}

func TestLinuxKillSwitch_NoRestoreWhenDeleteFailed(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip route del default via 192.168.1.1 dev eth0", "RTNETLINK answers: No such process")
	f.failOn("ip route add blackhole default", "RTNETLINK answers: File exists")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	if err := m.AddKillSwitchRoute(); err == nil {
		t.Fatal("AddKillSwitchRoute = nil; want an error")
	}
	if f.count("ip route add default via 192.168.1.1 dev eth0") != 0 {
		t.Fatalf("calls = %v; nothing was deleted, so nothing should be re-added", f.Calls())
	}
}

func TestLinuxSplitDefault_ErrorsWhenEitherHalfFails(t *testing.T) {
	for _, failing := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		t.Run(failing, func(t *testing.T) {
			f := useFakeRunner(t)
			f.failOn("ip route add "+failing+" dev tun0", "RTNETLINK answers: File exists")
			m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

			if err := m.AddSplitDefault(); err == nil {
				t.Fatalf("AddSplitDefault = nil with %s failing; want an error so startup aborts", failing)
			}
		})
	}
}

func TestLinuxSplitDefault_ErrorNamesManualDeleteHint(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip route add 0.0.0.0/1 dev tun0", "RTNETLINK answers: File exists")
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")

	err := m.AddSplitDefault()
	if err == nil || !strings.Contains(err.Error(), "sudo ip route del 0.0.0.0/1") {
		t.Fatalf("AddSplitDefault = %v; want a hint naming the manual delete command", err)
	}
}

func TestLinuxSplitDefault_NilWhenBothHalvesAdded(t *testing.T) {
	useFakeRunner(t)
	m := NewLinuxRouteManager("tun0", "192.168.1.1", "eth0")
	if err := m.AddSplitDefault(); err != nil {
		t.Fatalf("AddSplitDefault = %v; want nil", err)
	}
}
