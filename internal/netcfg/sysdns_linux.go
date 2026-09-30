//go:build linux

package netcfg

import (
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/safefile"
)

// resolvedOwnerPath records which process gave its TUN link the "~." DNS route; resolved
// itself keeps no trace of who set it. /run is cleared at boot, like the link settings.
var resolvedOwnerPath = "/run/urnet-client-dns.owner"

var haveTool = func(name string) bool {
	_, err := tools.resolve(name)
	return err == nil
}

// NewSystemDNS picks systemd-resolved when /etc/resolv.conf is its stub file: glibc then asks
// 127.0.0.53, and only resolved's per-link routing decides where queries go. Anything else
// gets /etc/resolv.conf rewritten.
func NewSystemDNS(tunName string) SystemDNS {
	if haveTool("resolvectl") && usesResolvedStub(resolvConfPath) {
		return &resolvedDNS{tun: tunName, owner: resolvedOwnerPath}
	}
	return &resolvConfDNS{path: resolvConfPath, backup: resolvConfBackupPath}
}

// RecoverStaleDNS undoes DNS changes left by a run that died before its cleanup, and refuses
// while the run that made them is still alive.
func RecoverStaleDNS() error {
	if err := recoverStaleResolved(resolvedOwnerPath); err != nil {
		return err
	}
	return (&resolvConfDNS{path: resolvConfPath, backup: resolvConfBackupPath}).recoverStale()
}

// recoverStaleResolved drops a dead run's owner file. Its per-link settings normally vanish
// with its TUN; the revert covers a TUN that outlived it.
func recoverStaleResolved(owner string) error {
	b, err := safefile.ReadFile(owner, 4<<10)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", owner, err)
	}
	if err := liveOwnerErr(parseOwnerPID(string(b))); err != nil {
		return err
	}
	if tun := ownerTUN(string(b)); tun != "" {
		_ = run("resolvectl", "revert", tun)
	}
	return os.Remove(owner)
}

func ownerTUN(s string) string {
	for _, f := range strings.Fields(s) {
		if v, ok := strings.CutPrefix(f, "tun="); ok {
			return v
		}
	}
	return ""
}

func usesResolvedStub(path string) bool {
	target, err := filepath.EvalSymlinks(path)
	return err == nil && strings.HasSuffix(target, "/systemd/resolve/stub-resolv.conf")
}

type resolvedDNS struct {
	tun, owner string
	applied    bool
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
	if err := safefile.WriteFile(r.owner, []byte(ownerRecord()+" tun="+r.tun+"\n")); err != nil {
		_ = run("resolvectl", "revert", r.tun)
		return fmt.Errorf("record DNS owner in %s: %w", r.owner, err)
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
	err := run("resolvectl", "revert", r.tun)
	if rmErr := os.Remove(r.owner); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
		err = errors.Join(err, rmErr)
	}
	return err
}
