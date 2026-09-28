//go:build unix

package netcfg

import (
	"io/fs"
	"syscall"
)

func rootOnly(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0 && fi.Mode().Perm()&0o022 == 0
}
