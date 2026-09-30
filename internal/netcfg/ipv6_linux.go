//go:build linux

package netcfg

import (
	"os"
	"strings"
)

var ipv6DisabledPath = "/proc/sys/net/ipv6/conf/all/disable_ipv6"

// IPv6Available reports whether the kernel has IPv6 enabled. Containers often run with it
// disabled; then there is nothing to leak and `ip -6 route` would fail.
func IPv6Available() bool {
	b, err := os.ReadFile(ipv6DisabledPath)
	return err == nil && strings.TrimSpace(string(b)) == "0"
}
