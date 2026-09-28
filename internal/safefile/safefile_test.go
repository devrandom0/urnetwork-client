package safefile

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func useEUID(t *testing.T, uid int) {
	t.Helper()
	old := geteuid
	geteuid = func() int { return uid }
	t.Cleanup(func() { geteuid = old })
}

func useOwner(t *testing.T, owner func(fs.FileInfo) (uint32, uint32, bool)) {
	t.Helper()
	old := fileOwner
	fileOwner = owner
	t.Cleanup(func() { fileOwner = old })
}

func recordChown(t *testing.T) *[][2]int {
	t.Helper()
	var calls [][2]int
	old := fchown
	fchown = func(_ *os.File, uid, gid int) error { calls = append(calls, [2]int{uid, gid}); return nil }
	t.Cleanup(func() { fchown = old })
	return &calls
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteFile_CreatesWith0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jwt")
	if err := WriteFile(path, []byte("tok\n")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 || mustRead(t, path) != "tok\n" {
		t.Fatalf("mode=%v err=%v", fi.Mode(), err)
	}
}

func TestWriteFile_ReplacesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	fi, _ := os.Stat(path)
	if mustRead(t, path) != "new" || fi.Mode().Perm() != 0o600 || len(entries) != 1 {
		t.Fatalf("content=%q mode=%v entries=%d", mustRead(t, path), fi.Mode(), len(entries))
	}
}

func TestWriteFile_RefusesSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "jwt")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(link, []byte("pwned")); err == nil {
		t.Fatal("WriteFile replaced a symlinked target")
	}
	if mustRead(t, victim) != "keep" {
		t.Fatal("symlink target was modified")
	}
}

func TestWriteFile_RefusesDirOwnedByAnotherUser(t *testing.T) {
	useEUID(t, 4242)
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 4343, 4343, true })
	if err := WriteFile(filepath.Join(t.TempDir(), "jwt"), []byte("x")); err == nil {
		t.Fatal("wrote into a directory owned by another non-root user")
	}
}

func TestWriteFile_RefusesGroupWritableDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(dir, "jwt"), []byte("x")); err == nil {
		t.Fatal("wrote into a group-writable directory without the sticky bit")
	}
}

func TestWriteFile_AllowsRootOwnedStickyDir(t *testing.T) {
	dir := t.TempDir()
	// os.Chmod's FileMode encodes ModeSticky as bit 20, not the raw unix 01000 octal bit,
	// so the literal 0o1777 silently drops the sticky bit; fs.ModeSticky sets it for real.
	if err := os.Chmod(dir, 0o777|fs.ModeSticky); err != nil {
		t.Fatal(err)
	}
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 0, 0, true })
	if err := WriteFile(filepath.Join(dir, "log"), []byte("x")); err != nil {
		t.Fatalf("a root-owned sticky dir like /tmp must be allowed: %v", err)
	}
}

func TestWriteFile_FollowsSymlinkedParentDir(t *testing.T) {
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(linkDir, "jwt"), []byte("x")); err != nil {
		t.Fatalf("a symlinked directory such as macOS /tmp must work: %v", err)
	}
	if mustRead(t, filepath.Join(realDir, "jwt")) != "x" {
		t.Fatal("file not written into the resolved directory")
	}
}

func TestWriteFile_RootChownsToDirOwner(t *testing.T) {
	useEUID(t, 0)
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 1234, 5678, true })
	calls := recordChown(t)
	if err := WriteFile(filepath.Join(t.TempDir(), "jwt"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0] != [2]int{1234, 5678} {
		t.Fatalf("chown calls = %v; a sudo run must leave the file owned by the directory owner", *calls)
	}
}

func TestWriteFile_RootInRootDirDoesNotChown(t *testing.T) {
	useEUID(t, 0)
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 0, 0, true })
	calls := recordChown(t)
	if err := WriteFile(filepath.Join(t.TempDir(), "jwt"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Fatalf("chown calls = %v, want none", *calls)
	}
}

func TestOpenAppend_CreatesAndAppends0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "urnet.log")
	for _, s := range []string{"a\n", "b\n"} {
		f, err := OpenAppend(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	fi, _ := os.Stat(path)
	if mustRead(t, path) != "a\nb\n" || fi.Mode().Perm() != 0o600 {
		t.Fatalf("content=%q mode=%v", mustRead(t, path), fi.Mode())
	}
}

func TestOpenAppend_RefusesSymlink(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "urnet.log")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(link); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend followed a symlink")
	}
}

func TestOpenAppend_RefusesHardLinkedFile(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig")
	if err := os.WriteFile(orig, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "urnet.log")
	if err := os.Link(orig, link); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(link); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend accepted a file with two hard links")
	}
}

func TestOpenAppend_RefusesNonRegularFile(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "urnet.log")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(fifo); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend accepted a FIFO")
	}
}

func TestOpenAppend_RefusesFileOwnedByAnotherUser(t *testing.T) {
	euid := uint32(os.Geteuid())
	useOwner(t, func(fi fs.FileInfo) (uint32, uint32, bool) {
		if fi.IsDir() {
			return euid, euid, true
		}
		return 4343, 4343, true
	})
	path := filepath.Join(t.TempDir(), "urnet.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(path); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend accepted a file owned by another user")
	}
}
