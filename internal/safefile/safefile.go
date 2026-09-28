// Package safefile writes files that may sit in directories another user controls, such as
// a user's home during a sudo run, without following symlinks planted there.
package safefile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

var (
	geteuid   = os.Geteuid
	fileOwner = statOwner
	fchown    = func(f *os.File, uid, gid int) error { return f.Chown(uid, gid) }
)

func statOwner(fi fs.FileInfo) (uid, gid uint32, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return st.Uid, st.Gid, true
}

// parseSudoUID returns -1 when SUDO_UID is unset or not a uid.
func parseSudoUID(s string) int {
	uid, err := strconv.Atoi(s)
	if err != nil || uid < 0 {
		return -1
	}
	return uid
}

// dir is a directory pinned by handle, so renaming or replacing its path after the checks
// cannot redirect later file operations.
type dir struct {
	root     *os.Root
	uid, gid uint32
}

func (d *dir) close() { _ = d.root.Close() }

func symlinkedPathErr(cleaned, actual string, euid int) error {
	if euid == 0 && actual != cleaned {
		return fmt.Errorf("%s resolves through a symlink to %s; refusing to follow it as root", cleaned, actual)
	}
	return nil
}

// parentDirErr refuses an ancestor whose owner could swap the directory below it.
func parentDirErr(path string, uid uint32, mode fs.FileMode, euid, sudoUID int) error {
	trusted := uid == 0 || int(uid) == euid || (euid == 0 && sudoUID >= 0 && int(uid) == sudoUID)
	if !trusted {
		return fmt.Errorf("%s is owned by uid %d, not root or the current user; refusing to write below it", path, uid)
	}
	if mode.Perm()&0o022 != 0 && (uid != 0 || mode&fs.ModeSticky == 0) {
		return fmt.Errorf("%s is writable by group or others; refusing to write below it", path)
	}
	return nil
}

func leafDirErr(path string, uid uint32, mode fs.FileMode, euid int) error {
	if euid != 0 && uid != 0 && int(uid) != euid {
		return fmt.Errorf("%s is owned by uid %d, not the current user (uid %d); refusing to write there", path, uid, euid)
	}
	if mode.Perm()&0o022 != 0 && (uid != 0 || mode&fs.ModeSticky == 0) {
		return fmt.Errorf("%s is writable by group or others; refusing to write there", path)
	}
	return nil
}

func ownerOf(fi fs.FileInfo, path string) (uid, gid uint32, err error) {
	uid, gid, ok := fileOwner(fi)
	if !ok {
		return 0, 0, fmt.Errorf("%s: cannot determine owner", path)
	}
	return uid, gid, nil
}

func checkParents(resolved string, euid, sudoUID int) error {
	for p := filepath.Dir(resolved); ; p = filepath.Dir(p) {
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%s is not a directory", p)
		}
		uid, _, err := ownerOf(fi, p)
		if err != nil {
			return err
		}
		if err := parentDirErr(p, uid, fi.Mode(), euid, sudoUID); err != nil {
			return err
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}

// openDir refuses a directory in which someone other than the current user or root could
// swap the target between our checks and our write, then pins it.
func openDir(path string) (*dir, error) {
	cleaned, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return nil, err
	}
	euid := geteuid()
	if err := symlinkedPathErr(cleaned, resolved, euid); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, err
	}
	d, err := checkPinned(root, path, resolved, euid)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return d, nil
}

func checkPinned(root *os.Root, path, resolved string, euid int) (*dir, error) {
	fi, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", path)
	}
	uid, gid, err := ownerOf(fi, path)
	if err != nil {
		return nil, err
	}
	if err := leafDirErr(path, uid, fi.Mode(), euid); err != nil {
		return nil, err
	}
	if err := checkParents(resolved, euid, parseSudoUID(os.Getenv("SUDO_UID"))); err != nil {
		return nil, err
	}
	onPath, err := os.Lstat(resolved)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(fi, onPath) {
		return nil, fmt.Errorf("%s changed while it was being checked", path)
	}
	return &dir{root: root, uid: uid, gid: gid}, nil
}

// chownTarget hands a file created by a sudo run to the invoking user, but only inside that
// user's own directory; anywhere else the file stays root-owned.
func chownTarget(euid int, dirUID, dirGID uint32, sudoUID int) (uid, gid int, ok bool) {
	if euid != 0 || dirUID == 0 || sudoUID < 0 || int(dirUID) != sudoUID {
		return 0, 0, false
	}
	return int(dirUID), int(dirGID), true
}

func chownForRoot(f *os.File, d *dir) error {
	uid, gid, ok := chownTarget(geteuid(), d.uid, d.gid, parseSudoUID(os.Getenv("SUDO_UID")))
	if !ok {
		return nil
	}
	return fchown(f, uid, gid)
}

func tempName(base string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "." + base + ".tmp-" + hex.EncodeToString(b[:]), nil
}

// WriteFile replaces path with data (mode 0600) through a temp file and rename, so readers
// never see a partial file and an existing symlink at path is never followed.
func WriteFile(path string, data []byte) error {
	d, err := openDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.close()
	name := filepath.Base(path)
	if fi, err := d.root.Lstat(name); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is a symlink or special file; refusing to replace it", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp, err := tempName(name)
	if err != nil {
		return err
	}
	f, err := d.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if err := fillTemp(f, data, d); err != nil {
		_ = f.Close()
		_ = d.root.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = d.root.Remove(tmp)
		return err
	}
	if err := d.root.Rename(tmp, name); err != nil {
		_ = d.root.Remove(tmp)
		return err
	}
	return nil
}

func fillTemp(f *os.File, data []byte, d *dir) error {
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if err := chownForRoot(f, d); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

// OpenAppend opens path for appending and creates it with mode 0600. It refuses symlinks,
// special files, hard-linked files and files owned by anyone but the current user or the
// directory owner.
func OpenAppend(path string) (*os.File, error) {
	d, err := openDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer d.close()
	name := filepath.Base(path)
	// O_NONBLOCK keeps a FIFO planted at path from blocking the open; it is cleared once the
	// descriptor is known to be a regular file.
	const flags = os.O_WRONLY | os.O_APPEND | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	created := true
	f, err := d.root.OpenFile(name, flags|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		created = false
		f, err = d.root.OpenFile(name, flags, 0)
	}
	if err != nil {
		return nil, err
	}
	if err := checkOpened(f, path, name, d, created); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func checkOpened(f *os.File, path, name string, d *dir, created bool) error {
	fi, err := checkRegular(f, path)
	if err != nil {
		return err
	}
	// os.Root follows symlinks that stay inside the directory even with O_NOFOLLOW, so the
	// opened file must be the entry itself.
	if entry, err := d.root.Lstat(name); err != nil || !os.SameFile(fi, entry) {
		return fmt.Errorf("%s is a symlink or was replaced while opening; refusing to append to it", path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return fmt.Errorf("%s has %d hard links; refusing to append to it", path, st.Nlink)
	}
	uid, _, err := ownerOf(fi, path)
	if err != nil {
		return err
	}
	if int(uid) != geteuid() && uid != d.uid {
		return fmt.Errorf("%s is owned by uid %d; refusing to append to it", path, uid)
	}
	if err := syscall.SetNonblock(int(f.Fd()), false); err != nil {
		return err
	}
	if created {
		return chownForRoot(f, d)
	}
	return nil
}

func checkRegular(f *os.File, path string) (fs.FileInfo, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return fi, nil
}

// ReadFile reads path, refusing symlinks, special files and files larger than limit bytes.
func ReadFile(path string, limit int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fi, err := checkRegular(f, path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, limit)
	}
	return data, nil
}
