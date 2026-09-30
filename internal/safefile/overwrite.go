//go:build unix

package safefile

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Overwrite replaces the contents of the existing regular file at path in place. Unlike
// WriteFile it keeps the inode: Docker bind-mounts /etc/resolv.conf, and a rename over a
// mount point fails with EBUSY. It never creates the file and never follows a symlink.
func Overwrite(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) {
		return fmt.Errorf("%s is a symlink; refusing to write through it", path)
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := checkRegular(f, path); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.WriteAt(data, 0); err != nil {
		return err
	}
	return f.Sync()
}
