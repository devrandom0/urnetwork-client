//go:build unix

package safefile

import (
	"io/fs"
	"os"
	"syscall"
)

const (
	noFollowFlag = syscall.O_NOFOLLOW
	nonblockFlag = syscall.O_NONBLOCK
)

func statOwner(fi fs.FileInfo) (uid, gid uint32, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return st.Uid, st.Gid, true
}

func nlinkOf(fi fs.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Nlink), true
}

func clearNonblock(f *os.File) error {
	return syscall.SetNonblock(int(f.Fd()), false)
}
