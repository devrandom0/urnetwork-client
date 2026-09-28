package safefile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func useEUID(t *testing.T, uid int) {
	t.Helper()
	old := geteuid
	geteuid = func() int { return uid }
	t.Cleanup(func() { geteuid = old })
}

// useOwner's func sees the pinned target directory as ".", its ancestors by their base names.
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

// realTempDir resolves t.TempDir, which sits under the /var -> /private/var symlink on macOS,
// so tests that act as root do not trip the symlinked-path refusal.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func withTimeout(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("call blocked")
	}
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
	if os.Geteuid() == 0 {
		useEUID(t, 4242)
	}
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

func TestWriteFile_RootChownsToSudoUserDir(t *testing.T) {
	useEUID(t, 0)
	t.Setenv("SUDO_UID", "1234")
	useOwner(t, func(fi fs.FileInfo) (uint32, uint32, bool) {
		if fi.Name() == "." {
			return 1234, 5678, true
		}
		return 0, 0, true
	})
	calls := recordChown(t)
	if err := WriteFile(filepath.Join(realTempDir(t), "jwt"), []byte("x")); err != nil {
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
	if err := WriteFile(filepath.Join(realTempDir(t), "jwt"), []byte("x")); err != nil {
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

func TestSymlinkedPathErr(t *testing.T) {
	cases := []struct {
		name            string
		cleaned, actual string
		euid            int
		wantErr         bool
	}{
		{"non-root may use a symlinked dir", "/tmp/x", "/private/tmp/x", 1000, false},
		{"root refuses a symlinked dir", "/tmp/x", "/private/tmp/x", 0, true},
		{"root accepts a plain dir", "/root/.urnetwork", "/root/.urnetwork", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := symlinkedPathErr(c.cleaned, c.actual, c.euid)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), c.cleaned) {
				t.Fatalf("error %q does not name the configured path", err)
			}
		})
	}
}

func TestWriteFile_RootRefusesSymlinkedDir(t *testing.T) {
	useEUID(t, 0)
	useOwner(t, func(fs.FileInfo) (uint32, uint32, bool) { return 0, 0, true })
	linkDir := filepath.Join(realTempDir(t), "home")
	if err := os.Symlink(realTempDir(t), linkDir); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(linkDir, "jwt"), []byte("x")); err == nil {
		t.Fatal("a root run wrote through a symlinked directory")
	}
}

func TestParentDirErr(t *testing.T) {
	cases := []struct {
		name          string
		uid           uint32
		mode          fs.FileMode
		euid, sudoUID int
		wantErr       bool
	}{
		{"root-owned 0755", 0, fs.ModeDir | 0o755, 1000, -1, false},
		{"owned by euid", 1000, fs.ModeDir | 0o755, 1000, -1, false},
		{"owned by another user", 2000, fs.ModeDir | 0o755, 1000, -1, true},
		{"owned by SUDO_UID under root", 1000, fs.ModeDir | 0o755, 0, 1000, false},
		{"owned by another user under root", 2000, fs.ModeDir | 0o755, 0, 1000, true},
		{"SUDO_UID ignored for non-root", 1000, fs.ModeDir | 0o755, 3000, 1000, true},
		{"group writable", 1000, fs.ModeDir | 0o775, 1000, -1, true},
		{"world writable sticky root-owned", 0, fs.ModeDir | fs.ModeSticky | 0o777, 1000, -1, false},
		{"world writable sticky user-owned", 1000, fs.ModeDir | fs.ModeSticky | 0o777, 1000, -1, true},
		{"world writable root-owned no sticky", 0, fs.ModeDir | 0o777, 1000, -1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := parentDirErr("/p", c.uid, c.mode, c.euid, c.sudoUID)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestWriteFile_RefusesGroupWritableParent(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "shared")
	leaf := filepath.Join(parent, "home")
	if err := os.MkdirAll(leaf, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(leaf, "jwt"), []byte("x")); err == nil {
		t.Fatal("wrote below a group-writable parent directory")
	}
	if f, err := OpenAppend(filepath.Join(leaf, "urnet.log")); err == nil {
		_ = f.Close()
		t.Fatal("appended below a group-writable parent directory")
	}
}

func TestChownTarget(t *testing.T) {
	cases := []struct {
		name    string
		euid    int
		dirUID  uint32
		sudoUID string
		wantOK  bool
	}{
		{"non-root never chowns", 1000, 1000, "1000", false},
		{"root chowns into the sudo user's dir", 0, 1000, "1000", true},
		{"root leaves files in another user's dir root-owned", 0, 2000, "1000", false},
		{"root without sudo leaves files root-owned", 0, 1000, "", false},
		{"unparsable SUDO_UID is ignored", 0, 1000, "abc", false},
		{"root-owned dir", 0, 0, "0", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uid, gid, ok := chownTarget(c.euid, c.dirUID, 77, parseSudoUID(c.sudoUID))
			if ok != c.wantOK || (ok && (uid != int(c.dirUID) || gid != 77)) {
				t.Fatalf("got uid=%d gid=%d ok=%v, want ok=%v", uid, gid, ok, c.wantOK)
			}
		})
	}
}

func TestOpenAppend_RefusesSymlinkInsideDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "other"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other", filepath.Join(dir, "urnet.log")); err != nil {
		t.Fatal(err)
	}
	if f, err := OpenAppend(filepath.Join(dir, "urnet.log")); err == nil {
		_ = f.Close()
		t.Fatal("OpenAppend followed a symlink to a file in the same directory")
	}
}

func TestOpenAppend_RefusesFIFOWithReaderWithoutHanging(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "urnet.log")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	withTimeout(t, 5*time.Second, func() {
		if f, err := OpenAppend(fifo); err == nil {
			_ = f.Close()
			t.Error("OpenAppend accepted a FIFO with a reader")
		}
	})
}

func TestOpenAppend_RefusesFIFOWithoutReaderWithoutHanging(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "urnet.log")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	withTimeout(t, 5*time.Second, func() {
		if f, err := OpenAppend(fifo); err == nil {
			_ = f.Close()
			t.Error("OpenAppend accepted a FIFO")
		}
	})
}

func TestOpenAppend_LeavesFileBlocking(t *testing.T) {
	f, err := OpenAppend(filepath.Join(t.TempDir(), "urnet.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if nonblocking(t, f) {
		t.Fatal("O_NONBLOCK still set on the returned log file")
	}
}

func TestReadFile_ReadsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := ReadFile(path, 16)
	if err != nil || string(b) != "hello" {
		t.Fatalf("got %q, %v", b, err)
	}
}

func TestReadFile_MissingFileIsNotExist(t *testing.T) {
	if _, err := ReadFile(filepath.Join(t.TempDir(), "nope"), 16); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotExist", err)
	}
}

func TestReadFile_RefusesSymlink(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "jwt")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(link, 16); err == nil {
		t.Fatal("ReadFile followed a symlink")
	}
}

func TestReadFile_RefusesOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path, 9); err == nil {
		t.Fatal("ReadFile accepted a file above the size cap")
	}
	if b, err := ReadFile(path, 10); err != nil || len(b) != 10 {
		t.Fatalf("file exactly at the cap: %q, %v", b, err)
	}
}

func TestReadFile_RefusesFIFOWithoutHanging(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "jwt")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	withTimeout(t, 5*time.Second, func() {
		if _, err := ReadFile(fifo, 16); err == nil {
			t.Error("ReadFile accepted a FIFO")
		}
	})
}

func nonblocking(t *testing.T, f *os.File) bool {
	t.Helper()
	rc, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags uintptr
	var errno syscall.Errno
	if err := rc.Control(func(fd uintptr) {
		flags, _, errno = syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
	}); err != nil {
		t.Fatal(err)
	}
	if errno != 0 {
		t.Fatal(errno)
	}
	return flags&syscall.O_NONBLOCK != 0
}
