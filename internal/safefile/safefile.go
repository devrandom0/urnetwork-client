// Package safefile writes files that may sit in directories another user controls, such as
// a user's home during a sudo run, without following symlinks planted there.
package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

type dirInfo struct {
	path     string
	uid, gid uint32
}

// safeDir refuses a directory in which someone other than the current user or root could
// swap the target between our checks and our write.
func safeDir(dir string) (dirInfo, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dirInfo{}, err
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return dirInfo{}, err
	}
	if !fi.IsDir() {
		return dirInfo{}, fmt.Errorf("%s is not a directory", dir)
	}
	uid, gid, ok := fileOwner(fi)
	if !ok {
		return dirInfo{}, fmt.Errorf("%s: cannot determine owner", dir)
	}
	euid := geteuid()
	if euid != 0 && uid != 0 && int(uid) != euid {
		return dirInfo{}, fmt.Errorf("%s is owned by uid %d, not the current user (uid %d); refusing to write there", dir, uid, euid)
	}
	if fi.Mode().Perm()&0o022 != 0 && fi.Mode()&fs.ModeSticky == 0 {
		return dirInfo{}, fmt.Errorf("%s is writable by group or others; refusing to write there", dir)
	}
	return dirInfo{path: resolved, uid: uid, gid: gid}, nil
}

// chownForRoot keeps a file created by a sudo run usable by the owner of the directory it is in.
func chownForRoot(f *os.File, d dirInfo) error {
	if geteuid() != 0 || d.uid == 0 {
		return nil
	}
	return fchown(f, int(d.uid), int(d.gid))
}

// WriteFile replaces path with data (mode 0600) through a temp file and rename, so readers
// never see a partial file and an existing symlink at path is never followed.
func WriteFile(path string, data []byte) error {
	d, err := safeDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	target := filepath.Join(d.path, filepath.Base(path))
	if fi, err := os.Lstat(target); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is a symlink or special file; refusing to replace it", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(d.path, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := fillTemp(tmp, data, d); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func fillTemp(f *os.File, data []byte, d dirInfo) error {
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
	d, err := safeDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	target := filepath.Join(d.path, filepath.Base(path))
	fi, err := os.Lstat(target)
	existed := err == nil
	if existed && !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is a symlink or special file; refusing to open it", path)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := checkOpened(f, path, d, existed); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func checkOpened(f *os.File, path string, d dirInfo, existed bool) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return fmt.Errorf("%s has %d hard links; refusing to append to it", path, st.Nlink)
	}
	uid, _, ok := fileOwner(fi)
	if !ok {
		return fmt.Errorf("%s: cannot determine owner", path)
	}
	if int(uid) != geteuid() && uid != d.uid {
		return fmt.Errorf("%s is owned by uid %d; refusing to append to it", path, uid)
	}
	if !existed {
		return chownForRoot(f, d)
	}
	return nil
}
