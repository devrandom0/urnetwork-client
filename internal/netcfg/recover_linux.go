//go:build linux

package netcfg

import (
	"fmt"
	"strings"
)

// Recover undoes what a run that died without cleanup left behind: resolv.conf or the
// systemd-resolved link first, then the IPv6 reject halves and split routes through a TUN
// that no longer exists. It reports each change it made.
func Recover(o RecoverOptions) ([]string, error) {
	markers, err := dnsMarkers()
	if err != nil {
		return nil, err
	}
	return recoverState(o, markers, removeStaleIPv6Rejects, removeOrphanSplitRoutes)
}

// removeStaleIPv6Rejects deletes ::/1 and 8000::/1 only when they are reject routes; the same
// halves through another VPN's interface belong to that VPN.
func removeStaleIPv6Rejects() ([]string, error) {
	var actions []string
	for _, dst := range []string{"::/1", "8000::/1"} {
		for _, f := range exactRoutes("-6", dst) {
			switch f[0] {
			case "unreachable", "prohibit", "blackhole":
			default:
				continue
			}
			if err := run("ip", "-6", "route", "del", f[0], dst); err != nil {
				return actions, fmt.Errorf("delete IPv6 reject route %s: %w", dst, err)
			}
			actions = append(actions, "removed the IPv6 reject route "+dst)
		}
	}
	return actions, nil
}

func removeOrphanSplitRoutes() ([]string, error) {
	var actions []string
	for _, dst := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		for _, f := range exactRoutes("-4", dst) {
			dev := fieldAfter(f, "dev")
			if dev == "" || interfaceExists(dev) {
				continue
			}
			if err := run("ip", "route", "del", dst, "dev", dev); err != nil {
				return actions, fmt.Errorf("delete route %s via missing %s: %w", dst, dev, err)
			}
			actions = append(actions, fmt.Sprintf("removed the route %s via %s, which no longer exists", dst, dev))
		}
	}
	return actions, nil
}

// exactRoutes returns the fields of each `ip route show exact` line. A failed lookup, such
// as with IPv6 off in the kernel, yields none: there is nothing to remove.
func exactRoutes(family, dst string) [][]string {
	args := []string{"route", "show", "exact", dst}
	if family == "-6" {
		args = append([]string{"-6"}, args...)
	}
	out, err := runCaptureStdout("ip", args...)
	if err != nil {
		return nil
	}
	var rows [][]string
	for _, ln := range strings.Split(out, "\n") {
		if f := strings.Fields(ln); len(f) > 0 {
			rows = append(rows, f)
		}
	}
	return rows
}

func fieldAfter(f []string, key string) string {
	for i := 0; i+1 < len(f); i++ {
		if f[i] == key {
			return f[i+1]
		}
	}
	return ""
}
