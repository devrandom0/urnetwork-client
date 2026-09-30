//go:build windows

package netcfg

import "io/fs"

// Windows has no root-owned-tool concept; the privileged-tool lookup this guards is unused
// since the VPN/route/TUN code it serves is unix-only, so this always refuses.
func rootOnly(fs.FileInfo) bool {
	return false
}
