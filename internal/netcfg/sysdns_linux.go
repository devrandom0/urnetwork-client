//go:build linux

package netcfg

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

var haveTool = func(name string) bool {
	_, err := tools.resolve(name)
	return err == nil
}

// NewSystemDNS picks systemd-resolved when /etc/resolv.conf is its stub file: glibc then asks
// 127.0.0.53, and only resolved's per-link routing decides where queries go. Anything else
// gets /etc/resolv.conf rewritten.
func NewSystemDNS(tunName string) SystemDNS {
	if haveTool("resolvectl") && usesResolvedStub(resolvConfPath) {
		return &resolvedDNS{tun: tunName}
	}
	return &resolvConfDNS{path: resolvConfPath, backup: resolvConfBackupPath}
}

// RecoverStaleDNS undoes a resolv.conf change left by a run that died before its cleanup.
// systemd-resolved needs nothing: its per-link settings vanish with the TUN.
func RecoverStaleDNS() error {
	return (&resolvConfDNS{path: resolvConfPath, backup: resolvConfBackupPath}).recoverStale()
}

func usesResolvedStub(path string) bool {
	target, err := filepath.EvalSymlinks(path)
	return err == nil && strings.HasSuffix(target, "/systemd/resolve/stub-resolv.conf")
}

type resolvedDNS struct {
	tun     string
	applied bool
}

// Apply makes the TUN link the catch-all ("~.") default route for DNS, so resolved sends
// every lookup without a more specific routing domain to the stub.
func (r *resolvedDNS) Apply(stub netip.Addr) error {
	steps := [][]string{
		{"dns", r.tun, stub.String()},
		{"domain", r.tun, "~."},
		{"default-route", r.tun, "true"},
	}
	for _, s := range steps {
		if err := run("resolvectl", s...); err != nil {
			_ = run("resolvectl", "revert", r.tun)
			return fmt.Errorf("resolvectl %s: %w", strings.Join(s, " "), err)
		}
	}
	r.applied = true
	logx.Info("DNS: systemd-resolved sends all lookups to the stub %s via %s\n", stub, r.tun)
	return nil
}

func (r *resolvedDNS) Restore() error {
	if !r.applied {
		return nil
	}
	r.applied = false
	return run("resolvectl", "revert", r.tun)
}
