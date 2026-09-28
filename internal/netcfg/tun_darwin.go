//go:build darwin

package netcfg

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// tunCIDRParts extracts the host IP and derives a peer/gateway IP from an ip/prefix CIDR.
// For "10.255.0.2/24" it returns ("10.255.0.2", "10.255.0.1").
func tunCIDRParts(ipCIDR string) (ip, peer string) {
	ip = ipCIDR
	peer = "10.255.0.1"
	if idx := strings.Index(ipCIDR, "/"); idx >= 0 {
		ip = ipCIDR[:idx]
	}
	if i := strings.LastIndex(ip, "."); i > 0 {
		peer = ip[:i] + ".1"
	}
	return
}

func runSudo(name string, args ...string) error {
	return cmdRunner.Run(name, args...)
}

// ConfigureDarwinTUN must succeed before any route points at the utun; a half-configured
// device would blackhole all routed traffic.
func ConfigureDarwinTUN(name, ipCIDR string, mtu int, enableIPv6 bool) (string, error) {
	tunIP, peerIP := tunCIDRParts(ipCIDR)
	if err := runSudo("ifconfig", name, "inet", tunIP, peerIP, "mtu", strconv.Itoa(mtu), "up"); err != nil {
		return "", fmt.Errorf("configure TUN %s: %w", name, err)
	}
	if err := runSudo("ifconfig", name, "inet6", "fd00::2/120"); err != nil {
		if enableIPv6 {
			return "", fmt.Errorf("configure TUN %s IPv6 address: %w", name, err)
		}
		logx.Debug("IPv6 address on %s not set (%v); continuing because --enable_ipv6 is off\n", name, err)
	}
	return peerIP, nil
}

// DefaultGateway returns the IPv4 default gateway and interface (e.g., 192.168.1.1, en0) on macOS.
func DefaultGateway() (string, string, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	// Don't attach Stdout/Stderr to avoid noisy output; capture instead
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("route get default failed: %w", err)
	}
	var gw, iface string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			gw = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		} else if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	if gw == "" {
		return "", "", fmt.Errorf("no default gateway found")
	}
	return gw, iface, nil
}

// SystemDNSResolvers parses `scutil --dns` and returns unique IPv4 resolver IPs.
func SystemDNSResolvers() ([]string, error) {
	out, err := runCapture("scutil", "--dns")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	seen := map[string]bool{}
	var res []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		// Lines look like: 'nameserver[0] : 192.168.1.1'
		if strings.HasPrefix(ln, "nameserver[") {
			parts := strings.Split(ln, ":")
			if len(parts) >= 2 {
				ip := strings.TrimSpace(parts[1])
				if ip != "" && strings.Count(ip, ".") == 3 && !seen[ip] {
					seen[ip] = true
					res = append(res, ip)
				}
			}
		}
	}
	return res, nil
}
