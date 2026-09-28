package netcfg

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeTool(t *testing.T, dir, name string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func newTestLookup(dirs ...string) *toolLookup {
	return &toolLookup{dirs: dirs, stat: os.Stat, cache: map[string]toolResult{}}
}

func TestSystemToolDirs_Order(t *testing.T) {
	want := []string{"/sbin", "/usr/sbin", "/bin", "/usr/bin", "/run/current-system/sw/bin"}
	if !slices.Equal(systemToolDirs, want) {
		t.Fatalf("systemToolDirs = %v, want %v", systemToolDirs, want)
	}
}

func TestToolLookup_FirstDirWins(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeTool(t, a, "route", 0o755)
	writeTool(t, b, "route", 0o755)
	got, err := newTestLookup(a, b).resolve("route")
	if err != nil || got != filepath.Join(a, "route") {
		t.Fatalf("resolve = %q, %v; want %s", got, err, filepath.Join(a, "route"))
	}
}

func TestToolLookup_NeverUsesPATH(t *testing.T) {
	onPath := t.TempDir()
	writeTool(t, onPath, "ip", 0o755)
	t.Setenv("PATH", onPath)
	if got, err := newTestLookup(t.TempDir()).resolve("ip"); err == nil {
		t.Fatalf("resolve found %q via PATH; privileged tools must come from fixed dirs only", got)
	}
}

func TestToolLookup_SkipsNonExecutableAndDirectories(t *testing.T) {
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	writeTool(t, a, "scutil", 0o644)
	if err := os.Mkdir(filepath.Join(b, "scutil"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTool(t, c, "scutil", 0o755)
	got, err := newTestLookup(a, b, c).resolve("scutil")
	if err != nil || got != filepath.Join(c, "scutil") {
		t.Fatalf("resolve = %q, %v; want the executable regular file in %s", got, err, c)
	}
}

func TestToolLookup_ResolvesOnce(t *testing.T) {
	a := t.TempDir()
	writeTool(t, a, "ifconfig", 0o755)
	n := 0
	l := &toolLookup{dirs: []string{a}, stat: func(p string) (fs.FileInfo, error) { n++; return os.Stat(p) }, cache: map[string]toolResult{}}
	for range 3 {
		if _, err := l.resolve("ifconfig"); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("stat called %d times, want 1", n)
	}
}

func TestToolLookup_RejectsNamesWithSlash(t *testing.T) {
	if _, err := newTestLookup("/bin").resolve("/bin/sh"); err == nil {
		t.Fatal("a path must not bypass the fixed directory list")
	}
}
