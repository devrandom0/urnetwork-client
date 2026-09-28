//go:build windows

package safefile

import (
	"errors"
	"io/fs"
	"os"
)

// Windows has no POSIX uid/gid or O_NOFOLLOW; the package's root-only checks never trigger
// here because os.Geteuid always returns -1, so these are compile-time stubs only.
const (
	noFollowFlag = 0
	nonblockFlag = 0
)

var errSymlinkLoop = errors.New("safefile: symlink loop (unused on windows)")

func statOwner(fs.FileInfo) (uid, gid uint32, ok bool) {
	return 0, 0, false
}

func nlinkOf(fs.FileInfo) (uint64, bool) {
	return 0, false
}

func clearNonblock(*os.File) error {
	return nil
}
