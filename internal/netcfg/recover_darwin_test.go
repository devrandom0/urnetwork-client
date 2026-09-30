//go:build darwin

package netcfg

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

const scutilShowScript = "show " + scutilService + "/DNS\n"

const netstatInet6WithRejects = `Routing tables

Internet6:
Destination                             Gateway                                 Flags               Netif Expire
::/1                                    ::1                                     UGRSc                 lo0
default                                 fe80::1%en0                             UGcg                  en0
::1                                     ::1                                     UHL                   lo0
8000::/1                                ::1                                     UGRSc                 lo0
`

const netstatInet6Foreign = `Routing tables

Internet6:
Destination                             Gateway                                 Flags               Netif Expire
::/1                                    fe80::1%utun4                           UGcg                utun4
`

const netstatInetOrphanSplits = `Routing tables

Internet:
Destination        Gateway            Flags               Netif Expire
0/1                link#23            UCS                 utun5
default            192.168.1.1        UGScg                 en0
128.0/1            link#23            UCS                 utun5
10.8/16            link#4             UCS                 utun2
`

func useInterfaces(t *testing.T, present ...string) {
	t.Helper()
	old := interfaceExists
	interfaceExists = func(name string) bool { return slices.Contains(present, name) }
	t.Cleanup(func() { interfaceExists = old })
}

func recoverFake(t *testing.T, show string) *fakeRunner {
	t.Helper()
	f := useFakeRunner(t)
	f.byInput[scutilShowScript] = fakeResult{out: show}
	useInterfaces(t, "en0", "lo0", "utun2", "utun4")
	return f
}

func indexOf(calls []string, want string) int {
	return slices.Index(calls, want)
}

func TestRecover_RemovesDeadOwnersDNSBeforeRoutes(t *testing.T) {
	f := recoverFake(t, scutilShowOwned)
	f.results["netstat -rn -f inet6"] = fakeResult{stdout: netstatInet6WithRejects}
	usePIDAlive(t, false)

	actions, err := Recover(RecoverOptions{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}

	calls := f.Calls()
	dns := indexOf(f.Inputs(), scutilRemoveScript)
	if dns < 0 {
		t.Fatalf("scutil inputs = %q; want the stale entry removed", f.Inputs())
	}
	v6a, v6b := indexOf(calls, "route -n delete -inet6 ::/1"), indexOf(calls, "route -n delete -inet6 8000::/1")
	if v6a < 0 || v6b < 0 {
		t.Fatalf("calls = %v; want both IPv6 reject halves deleted", calls)
	}
	if indexOf(calls, "scutil") > v6a {
		t.Fatalf("calls = %v; DNS must be restored before any route is touched", calls)
	}
	if len(actions) != 3 || !strings.Contains(actions[0], scutilService) || !strings.Contains(actions[1], "::/1") || !strings.Contains(actions[2], "8000::/1") {
		t.Fatalf("actions = %q; want the DNS entry then both IPv6 routes reported", actions)
	}
}

func TestRecover_NothingToRecover(t *testing.T) {
	f := recoverFake(t, "  No such key\n")

	actions, err := Recover(RecoverOptions{})
	if err != nil || len(actions) != 0 {
		t.Fatalf("Recover = %q, %v; want no actions on a clean host", actions, err)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c, "delete") {
			t.Fatalf("calls = %v; nothing may be deleted on a clean host", f.Calls())
		}
	}
}

func TestRecover_RefusesLiveOwnerWithoutForce(t *testing.T) {
	f := recoverFake(t, scutilShowOwned)
	f.results["netstat -rn -f inet6"] = fakeResult{stdout: netstatInet6WithRejects}
	usePIDAlive(t, true)

	actions, err := Recover(RecoverOptions{})

	var live *LiveOwnerError
	if !errors.As(err, &live) || live.PID != 4242 {
		t.Fatalf("Recover = %v; want a LiveOwnerError for pid 4242", err)
	}
	if len(actions) != 0 || len(f.Inputs()) != 1 || indexOf(f.Calls(), "route -n delete -inet6 ::/1") >= 0 {
		t.Fatalf("inputs = %q, calls = %v; a live owner's DNS and routes must stay", f.Inputs(), f.Calls())
	}
}

func TestRecover_ForceIgnoresLiveOwner(t *testing.T) {
	f := recoverFake(t, scutilShowOwned)
	usePIDAlive(t, true)

	if _, err := Recover(RecoverOptions{Force: true}); err != nil {
		t.Fatalf("Recover(force): %v", err)
	}
	if indexOf(f.Inputs(), scutilRemoveScript) < 0 {
		t.Fatalf("scutil inputs = %q; --force must remove a live owner's entry", f.Inputs())
	}
}

func TestRecover_OwnerMustNameTheMarker(t *testing.T) {
	for name, show := range map[string]string{
		"other owner": scutilShowOwned,
		"no entry":    "  No such key\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := recoverFake(t, show)
			f.results["netstat -rn -f inet6"] = fakeResult{stdout: netstatInet6WithRejects}
			usePIDAlive(t, false)

			_, err := Recover(RecoverOptions{Owner: 777})

			if !errors.Is(err, ErrNotOwner) {
				t.Fatalf("Recover = %v; want ErrNotOwner", err)
			}
			if len(f.Inputs()) != 1 || indexOf(f.Calls(), "route -n delete -inet6 ::/1") >= 0 {
				t.Fatalf("inputs = %q, calls = %v; another run's DNS and routes must stay", f.Inputs(), f.Calls())
			}
		})
	}
}

func TestRecover_OwnerActsEvenWhenItsPIDLooksAlive(t *testing.T) {
	f := recoverFake(t, scutilShowOwned)
	usePIDAlive(t, true)

	if _, err := Recover(RecoverOptions{Owner: 4242}); err != nil {
		t.Fatalf("Recover(owner): %v", err)
	}
	if indexOf(f.Inputs(), scutilRemoveScript) < 0 {
		t.Fatalf("scutil inputs = %q; the watchdog must act for its own dead parent even if the pid was reused", f.Inputs())
	}
}

func TestRecover_KeepsIPv6RoutesItDidNotInstall(t *testing.T) {
	f := recoverFake(t, "  No such key\n")
	f.results["netstat -rn -f inet6"] = fakeResult{stdout: netstatInet6Foreign}

	actions, err := Recover(RecoverOptions{})
	if err != nil || len(actions) != 0 || indexOf(f.Calls(), "route -n delete -inet6 ::/1") >= 0 {
		t.Fatalf("Recover = %q, %v, calls %v; a route via another VPN's interface must stay", actions, err, f.Calls())
	}
}

func TestRecover_RemovesSplitRoutesOfMissingTUN(t *testing.T) {
	f := recoverFake(t, "  No such key\n")
	f.results["netstat -rn -f inet"] = fakeResult{stdout: netstatInetOrphanSplits}

	actions, err := Recover(RecoverOptions{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	calls := f.Calls()
	for _, want := range []string{"route -n delete -net 0.0.0.0/1", "route -n delete -net 128.0.0.0/1"} {
		if indexOf(calls, want) < 0 {
			t.Fatalf("calls = %v; missing %q", calls, want)
		}
	}
	if len(actions) != 2 || !strings.Contains(actions[0], "utun5") {
		t.Fatalf("actions = %q; want both split routes reported", actions)
	}
}

func TestRecover_KeepsSplitRoutesOfLiveTUN(t *testing.T) {
	f := recoverFake(t, "  No such key\n")
	f.results["netstat -rn -f inet"] = fakeResult{stdout: strings.ReplaceAll(netstatInetOrphanSplits, "utun5", "utun4")}

	if actions, err := Recover(RecoverOptions{}); err != nil || len(actions) != 0 {
		t.Fatalf("Recover = %q, %v, calls %v; routes via an existing interface must stay", actions, err, f.Calls())
	}
}

func TestRecover_ReportsDNSFailureAndStillCleansRoutes(t *testing.T) {
	f := recoverFake(t, scutilShowOwned)
	f.byInput[scutilRemoveScript] = fakeResult{out: "Permission denied\n"}
	f.results["netstat -rn -f inet6"] = fakeResult{stdout: netstatInet6WithRejects}
	usePIDAlive(t, false)

	_, err := Recover(RecoverOptions{})

	if err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("Recover = %v; want the scutil failure reported", err)
	}
	if indexOf(f.Calls(), "route -n delete -inet6 ::/1") < 0 {
		t.Fatalf("calls = %v; a DNS failure must not stop the route cleanup", f.Calls())
	}
}

func TestRemoveScutilServiceErrorPointsAtRecover(t *testing.T) {
	f := useFakeRunner(t)
	f.byInput[scutilRemoveScript] = fakeResult{out: "Permission denied\n"}

	err := removeScutilService()

	if err == nil || !strings.Contains(err.Error(), "sudo urnet-client recover") {
		t.Fatalf("removeScutilService = %v; want the recover command named", err)
	}
}

func TestScutilDNS_FailedRollbackPointsAtRecover(t *testing.T) {
	f := useFakeRunner(t)
	f.results["scutil --dns"] = fakeResult{out: scutilDNSWithoutStub}
	f.byInput[scutilRemoveScript] = fakeResult{out: "Permission denied\n"}

	err := testScutilDNS().Apply(netip.MustParseAddr("10.255.0.2"))

	if err == nil || !strings.Contains(err.Error(), "sudo urnet-client recover") {
		t.Fatalf("Apply = %v; the entry stayed published, so the error must name the recover command", err)
	}
}
