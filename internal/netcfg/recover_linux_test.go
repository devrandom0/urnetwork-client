//go:build linux

package netcfg

import (
	"errors"
	"net/netip"
	"os"
	"slices"
	"strings"
	"testing"
)

func useInterfaces(t *testing.T, present ...string) {
	t.Helper()
	old := interfaceExists
	interfaceExists = func(name string) bool { return slices.Contains(present, name) }
	t.Cleanup(func() { interfaceExists = old })
}

func linuxRecoverFake(t *testing.T) *fakeRunner {
	t.Helper()
	f := useFakeRunner(t)
	useResolvConfIn(t, t.TempDir())
	useInterfaces(t, "eth0", "wg0")
	return f
}

func writeStaleResolvConf(t *testing.T, pid string) {
	t.Helper()
	if err := os.WriteFile(resolvConfPath, []byte("# urnet-client pid="+pid+"\nnameserver 10.255.0.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resolvConfBackupPath, []byte("nameserver 192.168.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scriptV6Rejects(f *fakeRunner) {
	f.results["ip -6 route show exact ::/1"] = fakeResult{stdout: "unreachable ::/1 dev lo metric 1024 pref medium\n"}
	f.results["ip -6 route show exact 8000::/1"] = fakeResult{stdout: "unreachable 8000::/1 dev lo metric 1024 pref medium\n"}
}

func TestRecover_RestoresResolvConfBeforeRoutes(t *testing.T) {
	f := linuxRecoverFake(t)
	writeStaleResolvConf(t, "4242")
	scriptV6Rejects(f)
	usePIDAlive(t, false)

	actions, err := Recover(RecoverOptions{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if got, _ := os.ReadFile(resolvConfPath); string(got) != "nameserver 192.168.1.1\n" {
		t.Fatalf("resolv.conf = %q; want the backup restored", got)
	}
	calls := f.Calls()
	for _, want := range []string{"ip -6 route del unreachable ::/1", "ip -6 route del unreachable 8000::/1"} {
		if !slices.Contains(calls, want) {
			t.Fatalf("calls = %v; missing %q", calls, want)
		}
	}
	if len(actions) != 3 || !strings.Contains(actions[0], resolvConfPath) || !strings.Contains(actions[1], "::/1") {
		t.Fatalf("actions = %q; want resolv.conf first, then both IPv6 routes", actions)
	}
}

func TestRecover_RevertsResolvedLinkOfDeadOwner(t *testing.T) {
	f := linuxRecoverFake(t)
	if err := os.WriteFile(resolvedOwnerPath, []byte("pid=4242 tun=urnet7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	usePIDAlive(t, false)

	actions, err := Recover(RecoverOptions{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if f.count("resolvectl revert urnet7") != 1 {
		t.Fatalf("calls = %v; want the recorded link reverted", f.Calls())
	}
	if _, err := os.Lstat(resolvedOwnerPath); !os.IsNotExist(err) {
		t.Fatal("owner file not removed")
	}
	if len(actions) != 1 || !strings.Contains(actions[0], "urnet7") {
		t.Fatalf("actions = %q; want the resolved revert reported", actions)
	}
}

func TestRecover_NothingToRecoverOnLinux(t *testing.T) {
	f := linuxRecoverFake(t)
	if err := os.WriteFile(resolvConfPath, []byte("nameserver 192.168.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	actions, err := Recover(RecoverOptions{})
	if err != nil || len(actions) != 0 {
		t.Fatalf("Recover = %q, %v; want nothing on a clean host", actions, err)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c, " del ") || strings.Contains(c, "revert") {
			t.Fatalf("calls = %v; nothing may change on a clean host", f.Calls())
		}
	}
}

func TestRecover_RefusesLiveOwnerOnLinux(t *testing.T) {
	f := linuxRecoverFake(t)
	writeStaleResolvConf(t, "4242")
	scriptV6Rejects(f)
	usePIDAlive(t, true)

	_, err := Recover(RecoverOptions{})

	var live *LiveOwnerError
	if !errors.As(err, &live) || live.PID != 4242 {
		t.Fatalf("Recover = %v; want a LiveOwnerError for pid 4242", err)
	}
	if _, err := os.Lstat(resolvConfBackupPath); err != nil {
		t.Fatal("a live owner's backup must stay")
	}
	if slices.Contains(f.Calls(), "ip -6 route del unreachable ::/1") {
		t.Fatalf("calls = %v; a live owner's routes must stay", f.Calls())
	}
}

func TestRecover_ForceIgnoresLiveOwnerOnLinux(t *testing.T) {
	linuxRecoverFake(t)
	writeStaleResolvConf(t, "4242")
	usePIDAlive(t, true)

	if _, err := Recover(RecoverOptions{Force: true}); err != nil {
		t.Fatalf("Recover(force): %v", err)
	}
	if got, _ := os.ReadFile(resolvConfPath); string(got) != "nameserver 192.168.1.1\n" {
		t.Fatalf("resolv.conf = %q; --force must restore it", got)
	}
}

func TestRecover_OwnerMustNameTheMarkerOnLinux(t *testing.T) {
	f := linuxRecoverFake(t)
	writeStaleResolvConf(t, "4242")
	scriptV6Rejects(f)
	usePIDAlive(t, false)

	_, err := Recover(RecoverOptions{Owner: 777})

	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("Recover = %v; want ErrNotOwner", err)
	}
	if _, err := os.Lstat(resolvConfBackupPath); err != nil {
		t.Fatal("another run's backup must stay")
	}
	if slices.Contains(f.Calls(), "ip -6 route del unreachable ::/1") {
		t.Fatalf("calls = %v; another run's routes must stay", f.Calls())
	}
}

func TestRecover_KeepsIPv6RoutesItDidNotInstallOnLinux(t *testing.T) {
	f := linuxRecoverFake(t)
	f.results["ip -6 route show exact ::/1"] = fakeResult{stdout: "::/1 dev wg0 metric 1024 pref medium\n"}

	if actions, err := Recover(RecoverOptions{}); err != nil || len(actions) != 0 {
		t.Fatalf("Recover = %q, %v, calls %v; another VPN's route must stay", actions, err, f.Calls())
	}
}

func TestRecover_RemovesSplitRoutesOfMissingTUNOnLinux(t *testing.T) {
	f := linuxRecoverFake(t)
	f.results["ip route show exact 0.0.0.0/1"] = fakeResult{stdout: "0.0.0.0/1 dev urnet0 scope link\n"}
	f.results["ip route show exact 128.0.0.0/1"] = fakeResult{stdout: "128.0.0.0/1 dev wg0 scope link\n"}

	actions, err := Recover(RecoverOptions{})
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if !slices.Contains(f.Calls(), "ip route del 0.0.0.0/1 dev urnet0") || slices.Contains(f.Calls(), "ip route del 128.0.0.0/1 dev wg0") {
		t.Fatalf("calls = %v; want only the route via the missing TUN deleted", f.Calls())
	}
	if len(actions) != 1 || !strings.Contains(actions[0], "urnet0") {
		t.Fatalf("actions = %q", actions)
	}
}

func TestResolvConfErrorsPointAtRecover(t *testing.T) {
	useResolvConfIn(t, t.TempDir())
	if err := os.WriteFile(resolvConfBackupPath, []byte("nameserver 192.168.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &resolvConfDNS{path: resolvConfPath, backup: resolvConfBackupPath}

	err := r.Apply(netip.MustParseAddr("10.255.0.2"))

	if err == nil || !strings.Contains(err.Error(), "sudo urnet-client recover") {
		t.Fatalf("Apply = %v; want the recover command named for a leftover backup", err)
	}
}

func TestResolvedDNS_RestoreFailurePointsAtRecover(t *testing.T) {
	f := useFakeRunner(t)
	r := testResolvedDNS(t)
	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatal(err)
	}
	f.failOn("resolvectl revert urnet0", "Failed to revert interface configuration")

	if err := r.Restore(); err == nil || !strings.Contains(err.Error(), "sudo urnet-client recover") {
		t.Fatalf("Restore = %v; want the recover command named", err)
	}
}

// A route listing that failed must not read as a clean host: the user would be told there
// is nothing to recover while the reject routes may still be there.
func TestRecover_WarnsWhenItCannotListRoutesOnLinux(t *testing.T) {
	f := linuxRecoverFake(t)
	if err := os.WriteFile(resolvConfPath, []byte("nameserver 192.168.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.failOn("ip -6 route show exact ::/1", "")

	var actions []string
	var err error
	logs := captureStderr(t, func() { actions, err = Recover(RecoverOptions{}) })
	if err != nil || len(actions) != 0 {
		t.Fatalf("Recover = %q, %v", actions, err)
	}
	if !strings.Contains(logs, "WARN") || !strings.Contains(logs, "ip -6 route show exact ::/1") || !strings.Contains(logs, "exit status 2") {
		t.Fatalf("logs = %q; want a WARN naming the failed ip command and its error", logs)
	}
}
