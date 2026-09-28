//go:build linux

package netcfg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

func run(name string, args ...string) error {
	return cmdRunner.Run(name, args...)
}

// ConfigureLinuxTUN must succeed before any route points at the TUN; a half-configured
// device would blackhole all routed traffic.
func ConfigureLinuxTUN(name, ipCIDR string, mtu int, enableIPv6 bool) error {
	steps := [][]string{
		{"ip", "addr", "add", ipCIDR, "dev", name},
		{"ip", "link", "set", "dev", name, "mtu", strconv.Itoa(mtu)},
		{"ip", "link", "set", name, "up"},
	}
	for _, s := range steps {
		if err := run(s[0], s[1:]...); err != nil {
			return fmt.Errorf("configure TUN %s (%s): %w", name, strings.Join(s, " "), err)
		}
	}
	if err := run("ip", "addr", "add", "fd00::2/120", "dev", name); err != nil {
		if enableIPv6 {
			return fmt.Errorf("configure TUN %s IPv6 address: %w", name, err)
		}
		logx.Debug("IPv6 address on %s not set (%v); continuing because --enable_ipv6 is off\n", name, err)
	}
	return nil
}

// DefaultRoute is a single default route entry from `ip route show default`.
type DefaultRoute struct {
	Gw, Dev string
	Metric  int
}

// ListDefaultRoutes parses all current default routes via `ip -o route show default`.
func ListDefaultRoutes() ([]DefaultRoute, error) {
	out, err := runCapture("ip", "-o", "route", "show", "default")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	routes := []DefaultRoute{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		toks := strings.Fields(line)
		var gw, dev string
		met := -1
		for i := 0; i < len(toks); i++ {
			switch toks[i] {
			case "via":
				if i+1 < len(toks) {
					gw = toks[i+1]
					i++
				}
			case "dev":
				if i+1 < len(toks) {
					dev = toks[i+1]
					i++
				}
			case "metric":
				if i+1 < len(toks) {
					if v, e := strconv.Atoi(toks[i+1]); e == nil {
						met = v
					}
					i++
				}
			}
		}
		if dev == "" {
			continue
		}
		routes = append(routes, DefaultRoute{Gw: gw, Dev: dev, Metric: met})
	}
	return routes, nil
}
