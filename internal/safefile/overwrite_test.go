//go:build unix

package safefile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOverwrite_ReplacesContentInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 192.168.1.1\nsearch lan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)

	if err := Overwrite(path, []byte("nameserver 10.255.0.2\n")); err != nil {
		t.Fatalf("Overwrite: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "nameserver 10.255.0.2\n" {
		t.Fatalf("content = %q", got)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Fatal("Overwrite replaced the inode; a bind-mounted file would fail with EBUSY")
	}
}

func TestOverwrite_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.WriteFile(target, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "resolv.conf")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := Overwrite(link, []byte("nameserver 10.255.0.2\n")); err == nil {
		t.Fatal("Overwrite wrote through a symlink")
	}
	if got, _ := os.ReadFile(target); string(got) != "keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestOverwrite_DoesNotCreateMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := Overwrite(path, []byte("x")); err == nil {
		t.Fatal("Overwrite created a missing file")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("Lstat = %v; want not-exist", err)
	}
}

func TestOverwrite_RefusesFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	if err := Overwrite(path, []byte("x")); err == nil {
		t.Fatal("Overwrite accepted a FIFO")
	}
}
