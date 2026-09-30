//go:build linux

package netcfg

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func useResolvConfIn(t *testing.T, dir string) {
	t.Helper()
	oldPath, oldBackup := resolvConfPath, resolvConfBackupPath
	resolvConfPath = filepath.Join(dir, "etc", "resolv.conf")
	resolvConfBackupPath = resolvConfPath + ".urnet-client.bak"
	t.Cleanup(func() { resolvConfPath, resolvConfBackupPath = oldPath, oldBackup })
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

func TestResolvedDNS_ApplyAndRestoreCommands(t *testing.T) {
	f := useFakeRunner(t)
	r := &resolvedDNS{tun: "urnet0"}

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
