package netcfg

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"testing"
)

func writeTool(t *testing.T, dir, name string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
}

type ownedInfo struct {
	fs.FileInfo
	uid uint32
}

func (o ownedInfo) Sys() any { return &syscall.Stat_t{Uid: o.uid} }

// statAs reports every file as owned by uid, so temp dirs can stand in for root-owned system dirs.
func statAs(uid uint32) func(string) (fs.FileInfo, error) {
	return func(p string) (fs.FileInfo, error) {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		return ownedInfo{FileInfo: fi, uid: uid}, nil
	}
}

func newTestLookup(dirs ...string) *toolLookup {
	return &toolLookup{dirs: dirs, stat: statAs(0), cache: map[string]toolResult{}}
}

func TestSystemToolDirs_Order(t *testing.T) {
	want := []string{"/sbin", "/usr/sbin", "/bin", "/usr/bin"}
	if runtime.GOOS == "linux" {
		want = append(want, "/run/current-system/sw/bin")
	}
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
	stat := statAs(0)
	l := &toolLookup{dirs: []string{a}, stat: func(p string) (fs.FileInfo, error) { n++; return stat(p) }, cache: map[string]toolResult{}}
	if _, err := l.resolve("ifconfig"); err != nil {
		t.Fatal(err)
	}
	first := n
	for range 2 {
		if _, err := l.resolve("ifconfig"); err != nil {
			t.Fatal(err)
		}
	}
	if n != first {
		t.Fatalf("stat called %d more times after the first resolve, want 0", n-first)
	}
}

func TestToolLookup_RejectsNamesWithSlash(t *testing.T) {
	if _, err := newTestLookup("/bin").resolve("/bin/sh"); err == nil {
		t.Fatal("a path must not bypass the fixed directory list")
	}
}

func TestToolLookup_SkipsToolNotOwnedByRoot(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeTool(t, a, "ip", 0o755)
	writeTool(t, b, "ip", 0o755)
	stat := func(p string) (fs.FileInfo, error) {
		if p == filepath.Join(a, "ip") {
			return statAs(1000)(p)
		}
		return statAs(0)(p)
	}
	l := &toolLookup{dirs: []string{a, b}, stat: stat, cache: map[string]toolResult{}}
	got, err := l.resolve("ip")
	if err != nil || got != filepath.Join(b, "ip") {
		t.Fatalf("resolve = %q, %v; want the root-owned tool in %s", got, err, b)
	}
}

func TestToolLookup_SkipsToolInDirNotOwnedByRoot(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeTool(t, a, "ip", 0o755)
	writeTool(t, b, "ip", 0o755)
	stat := func(p string) (fs.FileInfo, error) {
		if p == a {
			return statAs(1000)(p)
		}
		return statAs(0)(p)
	}
	l := &toolLookup{dirs: []string{a, b}, stat: stat, cache: map[string]toolResult{}}
	got, err := l.resolve("ip")
	if err != nil || got != filepath.Join(b, "ip") {
		t.Fatalf("resolve = %q, %v; want the tool whose dir is root-owned", got, err)
	}
}

func TestToolLookup_SkipsGroupOrWorldWritable(t *testing.T) {
	for _, mode := range []os.FileMode{0o775, 0o757} {
		a, b := t.TempDir(), t.TempDir()
		writeTool(t, a, "ip", 0o755)
		if err := os.Chmod(filepath.Join(a, "ip"), mode); err != nil {
			t.Fatal(err)
		}
		writeTool(t, b, "ip", 0o755)
		got, err := newTestLookup(a, b).resolve("ip")
		if err != nil || got != filepath.Join(b, "ip") {
			t.Fatalf("mode %o: resolve = %q, %v; want %s", mode, got, err, filepath.Join(b, "ip"))
		}
	}
}

func TestToolLookup_SkipsToolInWritableDir(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeTool(t, a, "ip", 0o755)
	writeTool(t, b, "ip", 0o755)
	if err := os.Chmod(a, 0o777); err != nil {
		t.Fatal(err)
	}
	got, err := newTestLookup(a, b).resolve("ip")
	if err != nil || got != filepath.Join(b, "ip") {
		t.Fatalf("resolve = %q, %v; want the tool in the non-writable dir", got, err)
	}
}

func TestToolLookup_ChecksSymlinkTargetDir(t *testing.T) {
	a, real, b := t.TempDir(), t.TempDir(), t.TempDir()
	writeTool(t, real, "ip", 0o755)
	if err := os.Symlink(filepath.Join(real, "ip"), filepath.Join(a, "ip")); err != nil {
		t.Fatal(err)
	}
	writeTool(t, b, "ip", 0o755)
	realResolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	stat := func(p string) (fs.FileInfo, error) {
		if p == realResolved {
			return statAs(1000)(p)
		}
		return statAs(0)(p)
	}
	l := &toolLookup{dirs: []string{a, b}, stat: stat, cache: map[string]toolResult{}}
	got, err := l.resolve("ip")
	if err != nil || got != filepath.Join(b, "ip") {
		t.Fatalf("resolve = %q, %v; a symlink into a user-owned dir must be skipped", got, err)
	}
}
