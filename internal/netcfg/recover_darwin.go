//go:build darwin

package netcfg

import (
	"fmt"
	"slices"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// Recover undoes what a run that died without cleanup left behind: the scutil DNS entry
// first, then the IPv6 reject halves and split routes through a TUN that no longer exists.
// It reports each change it made.
func Recover(o RecoverOptions) ([]string, error) {
	markers, err := dnsMarkers()
	if err != nil {
		return nil, err
	}
	return recoverState(o, markers, removeStaleIPv6Rejects, removeOrphanSplitRoutes)
}

// removeStaleIPv6Rejects deletes ::/1 and 8000::/1 only when they are reject or blackhole
// routes; the same halves through another VPN's interface belong to that VPN.
func removeStaleIPv6Rejects() ([]string, error) {
	var actions []string
	for _, r := range netstatRoutes("inet6", "::/1", "8000::/1") {
		if !strings.ContainsAny(r.flags, "RB") {
			continue
		}
		if err := runSudo("route", "-n", "delete", "-inet6", r.dest); err != nil {
			return actions, fmt.Errorf("delete IPv6 reject route %s: %w", r.dest, err)
		}
		actions = append(actions, "removed the IPv6 reject route "+r.dest)
	}
	return actions, nil
}

// netstatSplitCIDR maps netstat's abbreviated destinations to the split-default halves.
var netstatSplitCIDR = map[string]string{"0/1": "0.0.0.0/1", "128.0/1": "128.0.0.0/1"}

func removeOrphanSplitRoutes() ([]string, error) {
	var actions []string
	for _, r := range netstatRoutes("inet", "0/1", "128.0/1") {
		if interfaceExists(r.netif) {
			continue
		}
		cidr := netstatSplitCIDR[r.dest]
		if err := runSudo("route", "-n", "delete", "-net", cidr); err != nil {
			return actions, fmt.Errorf("delete route %s via missing %s: %w", cidr, r.netif, err)
		}
		actions = append(actions, fmt.Sprintf("removed the route %s via %s, which no longer exists", cidr, r.netif))
	}
	return actions, nil
}

type netstatRoute struct{ dest, flags, netif string }

// netstatRoutes lists the `netstat -rn` rows of one family whose destination is in dests.
// A failed netstat yields no rows, with a warning, so "nothing to recover" is not trusted.
func netstatRoutes(family string, dests ...string) []netstatRoute {
	out, err := runCaptureStdout("netstat", "-rn", "-f", family)
	if err != nil {
		logx.Warn("recover: could not list routes with `netstat -rn -f %s`: %v; leftover routes, if any, were not checked\n", family, err)
		return nil
	}
	var rows []netstatRoute
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) < 4 || !slices.Contains(dests, f[0]) {
			continue
		}
		rows = append(rows, netstatRoute{dest: f[0], flags: f[2], netif: f[3]})
	}
	return rows
}
