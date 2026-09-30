//go:build linux

package netcfg

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const origResolvConf = "nameserver 192.168.1.1\nsearch lan\n"

func tempResolvConf(t *testing.T) *resolvConfDNS {
	t.Helper()
	dir := t.TempDir()
	r := &resolvConfDNS{path: filepath.Join(dir, "resolv.conf"), backup: filepath.Join(dir, "resolv.conf.urnet-client.bak")}
	if err := os.WriteFile(r.path, []byte(origResolvConf), 0o644); err != nil {
		t.Fatal(err)
	}
	return r
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestResolvConf_ApplyWritesStubAndBackup(t *testing.T) {
	r := tempResolvConf(t)

	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got := readTestFile(t, r.path); !strings.Contains(got, "nameserver 10.255.0.2\n") || strings.Contains(got, "192.168.1.1") {
		t.Fatalf("resolv.conf = %q; want only the stub as nameserver", got)
	}
	if got := readTestFile(t, r.backup); got != origResolvConf {
		t.Fatalf("backup = %q; want the original", got)
	}
}

func TestResolvConf_RestorePutsOriginalBackAndDropsBackup(t *testing.T) {
	r := tempResolvConf(t)
	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatal(err)
	}

	if err := r.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := readTestFile(t, r.path); got != origResolvConf {
		t.Fatalf("resolv.conf = %q; want the original back", got)
	}
	if _, err := os.Lstat(r.backup); !os.IsNotExist(err) {
		t.Fatal("backup still exists after Restore; the next start would treat it as a crash leftover")
	}
	if err := r.Restore(); err != nil {
		t.Fatalf("second Restore = %v; want a no-op", err)
	}
}

func TestResolvConf_ApplyRefusesWhenBackupExists(t *testing.T) {
	r := tempResolvConf(t)
	if err := os.WriteFile(r.backup, []byte("nameserver 1.2.3.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err == nil {
		t.Fatal("Apply overwrote an existing backup; the real original would be lost")
	}
	if got := readTestFile(t, r.path); got != origResolvConf {
		t.Fatalf("resolv.conf changed to %q", got)
	}
}

func TestResolvConf_ApplyRefusesSymlink(t *testing.T) {
	r := tempResolvConf(t)
	managed := filepath.Join(filepath.Dir(r.path), "managed.conf")
	if err := os.Rename(r.path, managed); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(managed, r.path); err != nil {
		t.Fatal(err)
	}

	err := r.Apply(netip.MustParseAddr("10.255.0.2"))

	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Apply = %v; want a symlink error", err)
	}
	if _, err := os.Lstat(r.backup); !os.IsNotExist(err) {
		t.Fatal("backup written although Apply failed")
	}
}

func TestResolvConf_RecoverStaleRestoresLeftover(t *testing.T) {
	r := tempResolvConf(t)
	if err := r.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatal(err)
	}
	crashed := &resolvConfDNS{path: r.path, backup: r.backup}

	if err := crashed.recoverStale(); err != nil {
		t.Fatalf("recoverStale: %v", err)
	}

	if got := readTestFile(t, r.path); got != origResolvConf {
		t.Fatalf("resolv.conf = %q; want the original restored", got)
	}
	if _, err := os.Lstat(r.backup); !os.IsNotExist(err) {
		t.Fatal("backup still exists after recovery")
	}
}

func TestResolvConf_RecoverStaleNoopWithoutBackup(t *testing.T) {
	r := tempResolvConf(t)
	if err := r.recoverStale(); err != nil {
		t.Fatalf("recoverStale = %v; want nil", err)
	}
	if got := readTestFile(t, r.path); got != origResolvConf {
		t.Fatalf("resolv.conf changed to %q", got)
	}
}
