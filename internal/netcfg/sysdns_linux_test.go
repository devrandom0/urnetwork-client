//go:build linux

package netcfg

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useResolvConfIn(t *testing.T, dir string) {
	t.Helper()
	oldPath, oldBackup := resolvConfPath, resolvConfBackupPath
	resolvConfPath = filepath.Join(dir, "etc", "resolv.conf")
	resolvConfBackupPath = resolvConfPath + ".urnet-client.bak"
	oldOwner := resolvedOwnerPath
	resolvedOwnerPath = filepath.Join(dir, "run", "urnet-client-dns.owner")
	t.Cleanup(func() { resolvConfPath, resolvConfBackupPath, resolvedOwnerPath = oldPath, oldBackup, oldOwner })
	if err := os.MkdirAll(filepath.Dir(resolvedOwnerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(resolvConfPath), 0o755); err != nil {
		t.Fatal(err)
	}
}

func useHaveTool(t *testing.T, present bool) {
	t.Helper()
	old := haveTool
	haveTool = func(string) bool { return present }
	t.Cleanup(func() { haveTool = old })
}

func linkResolvedStub(t *testing.T, dir string) {
	t.Helper()
	stub := filepath.Join(dir, "run", "systemd", "resolve", "stub-resolv.conf")
	if err := os.MkdirAll(filepath.Dir(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stub, []byte("nameserver 127.0.0.53\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stub, resolvConfPath); err != nil {
		t.Fatal(err)
	}
}

func TestNewSystemDNS_UsesResolvedForItsStubFile(t *testing.T) {
	dir := t.TempDir()
	useResolvConfIn(t, dir)
	useHaveTool(t, true)
	linkResolvedStub(t, dir)

	if _, ok := NewSystemDNS("urnet0").(*resolvedDNS); !ok {
		t.Fatal("want systemd-resolved mode when resolv.conf is resolved's stub file")
	}
}

func TestNewSystemDNS_RewritesPlainResolvConf(t *testing.T) {
	dir := t.TempDir()
	useResolvConfIn(t, dir)
	useHaveTool(t, true)
	if err := os.WriteFile(resolvConfPath, []byte("nameserver 192.168.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, ok := NewSystemDNS("urnet0").(*resolvConfDNS); !ok {
		t.Fatal("want resolv.conf mode for a plain file even when resolvectl exists")
	}
}

func TestNewSystemDNS_NeedsResolvectlForResolvedMode(t *testing.T) {
	dir := t.TempDir()
	useResolvConfIn(t, dir)
	useHaveTool(t, false)
	linkResolvedStub(t, dir)

	if _, ok := NewSystemDNS("urnet0").(*resolvConfDNS); !ok {
		t.Fatal("without resolvectl the stub file cannot be managed through resolved")
	}
}

func testResolvedDNS(t *testing.T) *resolvedDNS {
	t.Helper()
	useResolvConfIn(t, t.TempDir())
	return &resolvedDNS{tun: "urnet0", owner: resolvedOwnerPath}
}

func TestResolvedDNS_ApplyAndRestoreCommands(t *testing.T) {
	f := useFakeRunner(t)
	r := testResolvedDNS(t)

	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := r.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	want := []string{
		"resolvectl dns urnet0 10.255.0.2",
		"resolvectl domain urnet0 ~.",
		"resolvectl default-route urnet0 true",
		"resolvectl domain",
		"resolvectl revert urnet0",
	}
	got := f.Calls()
	if len(got) != len(want) {
		t.Fatalf("calls = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call %d = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestResolvedDNS_ApplyFailureRevertsAndErrors(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("resolvectl domain urnet0 ~.", "Failed to set domain configuration: Unit dbus-org.freedesktop.resolve1.service not found.")
	r := &resolvedDNS{tun: "urnet0"}

	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err == nil {
		t.Fatal("Apply = nil; want an error so startup aborts")
	}
	if f.count("resolvectl revert urnet0") != 1 {
		t.Fatalf("calls = %v; want the half-applied link settings reverted", f.Calls())
	}
	if err := r.Restore(); err != nil || f.count("resolvectl revert urnet0") != 1 {
		t.Fatal("Restore after a failed Apply must be a no-op")
	}
}

func TestRecoverStaleDNS_RestoresResolvConfBackup(t *testing.T) {
	useResolvConfIn(t, t.TempDir())
	if err := os.WriteFile(resolvConfPath, []byte("nameserver 10.255.0.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resolvConfBackupPath, []byte("nameserver 192.168.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RecoverStaleDNS(); err != nil {
		t.Fatalf("RecoverStaleDNS: %v", err)
	}

	if got, _ := os.ReadFile(resolvConfPath); string(got) != "nameserver 192.168.1.1\n" {
		t.Fatalf("resolv.conf = %q; want the backup restored", got)
	}
}

func TestResolvedDNS_OwnerFileLivesForTheSession(t *testing.T) {
	useFakeRunner(t)
	r := testResolvedDNS(t)

	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, err := os.ReadFile(r.owner)
	if err != nil || string(got) != fmt.Sprintf("pid=%d tun=urnet0\n", os.Getpid()) {
		t.Fatalf("owner file = %q, %v; want this pid and the TUN", got, err)
	}
	if err := r.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Lstat(r.owner); !os.IsNotExist(err) {
		t.Fatal("owner file still exists after Restore; the next start would see a leftover")
	}
}

func TestRecoverStaleDNS_RefusesLiveResolvedOwner(t *testing.T) {
	f := useFakeRunner(t)
	useResolvConfIn(t, t.TempDir())
	if err := os.WriteFile(resolvedOwnerPath, []byte("pid=4242 tun=urnet7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	usePIDAlive(t, true)

	err := RecoverStaleDNS()

	if err == nil || !strings.Contains(err.Error(), "another urnet-client (pid 4242) owns system DNS") {
		t.Fatalf("RecoverStaleDNS = %v; want a refusal naming the live owner", err)
	}
	if len(f.Calls()) != 0 {
		t.Fatalf("calls = %v; a live instance's link settings must stay", f.Calls())
	}
}

func TestRecoverStaleDNS_RevertsDeadResolvedOwner(t *testing.T) {
	f := useFakeRunner(t)
	useResolvConfIn(t, t.TempDir())
	if err := os.WriteFile(resolvedOwnerPath, []byte("pid=4242 tun=urnet7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	usePIDAlive(t, false)

	if err := RecoverStaleDNS(); err != nil {
		t.Fatalf("RecoverStaleDNS: %v", err)
	}
	if f.count("resolvectl revert urnet7") != 1 {
		t.Fatalf("calls = %v; want the dead owner's link reverted", f.Calls())
	}
	if _, err := os.Lstat(resolvedOwnerPath); !os.IsNotExist(err) {
		t.Fatal("stale owner file not removed")
	}
}

func TestResolvedDNS_WarnsAboutOtherLinksRoutingDomains(t *testing.T) {
	f := useFakeRunner(t)
	f.results["resolvectl domain"] = fakeResult{stdout: "Global:\nLink 2 (eth0): lan ~corp.example\nLink 3 (wlan0):\nLink 7 (urnet0): ~.\n"}
	r := testResolvedDNS(t)

	logged := captureStderr(t, func() {
		if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
			t.Error(err)
		}
	})

	if !strings.Contains(logged, "eth0: lan ~corp.example") {
		t.Fatalf("stderr = %q; want a WARN naming the link and domains that bypass the stub", logged)
	}
	if strings.Contains(logged, "urnet0") || strings.Contains(logged, "wlan0") {
		t.Fatalf("stderr = %q; only other links with domains matter", logged)
	}
}

func TestResolvedDNS_NoWarningWithoutOtherDomains(t *testing.T) {
	f := useFakeRunner(t)
	f.results["resolvectl domain"] = fakeResult{stdout: "Global:\nLink 2 (eth0):\nLink 7 (urnet0): ~.\n"}
	r := testResolvedDNS(t)

	logged := captureStderr(t, func() {
		if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
			t.Error(err)
		}
	})

	if strings.Contains(logged, "WARN") {
		t.Fatalf("stderr = %q; want no warning", logged)
	}
}
