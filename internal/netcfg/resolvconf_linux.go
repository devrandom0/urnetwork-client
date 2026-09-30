//go:build linux

package netcfg

import (
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/safefile"
)

var (
	resolvConfPath       = "/etc/resolv.conf"
	resolvConfBackupPath = "/etc/resolv.conf.urnet-client.bak"
)

const maxResolvConfSize = 64 << 10

// resolvConfDNS rewrites /etc/resolv.conf for hosts and containers without systemd-resolved.
// The backup file is the crash marker: while it exists, resolv.conf is ours.
type resolvConfDNS struct {
	path, backup string
	applied      bool
}

func (r *resolvConfDNS) Apply(stub netip.Addr) error {
	if _, err := os.Lstat(r.backup); err == nil {
		return fmt.Errorf("%s exists from an earlier run; restore it with: sudo cp %s %s && sudo rm %s", r.backup, r.backup, r.path, r.backup)
	}
	if target, err := os.Readlink(r.path); err == nil {
		return fmt.Errorf("%s is a symlink to %s, managed by another DNS tool; urnet-client supports systemd-resolved (stub-resolv.conf) or a plain file", r.path, target)
	}
	orig, err := safefile.ReadFile(r.path, maxResolvConfSize)
	if err != nil {
		return fmt.Errorf("read %s: %w", r.path, err)
	}
	if err := safefile.WriteFile(r.backup, orig); err != nil {
		return fmt.Errorf("back up %s: %w", r.path, err)
	}
	content := fmt.Sprintf("# Written by urnet-client for this VPN session; the original is in %s\nnameserver %s\n", r.backup, stub)
	if err := safefile.Overwrite(r.path, []byte(content)); err != nil {
		return r.recoverFailedWrite(err, orig)
	}
	r.applied = true
	logx.Info("DNS: %s points at the stub %s\n", r.path, stub)
	return nil
}

// recoverFailedWrite runs when writing the DNS stub into r.path fails; safefile.Overwrite may
// have already truncated r.path before the failure, so the backup is the only copy of the
// original left. It puts the original back before dropping the backup, and only drops the
// backup once that restore is confirmed, so a crash never leaves both r.path and the backup bad.
func (r *resolvConfDNS) recoverFailedWrite(writeErr error, orig []byte) error {
	if err := safefile.Overwrite(r.path, orig); err != nil {
		return fmt.Errorf("point %s at the DNS stub: %w; the original is preserved in %s", r.path, writeErr, r.backup)
	}
	_ = os.Remove(r.backup)
	return fmt.Errorf("point %s at the DNS stub: %w", r.path, writeErr)
}

func (r *resolvConfDNS) Restore() error {
	if !r.applied {
		return nil
	}
	r.applied = false
	return r.restoreBackup()
}

// recoverStale restores a backup left by a run that died before Restore.
func (r *resolvConfDNS) recoverStale() error {
	if _, err := os.Lstat(r.backup); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	logx.Warn("restoring %s from %s, left by an earlier urnet-client run\n", r.path, r.backup)
	return r.restoreBackup()
}

func (r *resolvConfDNS) restoreBackup() error {
	orig, err := safefile.ReadFile(r.backup, maxResolvConfSize)
	if err != nil {
		return fmt.Errorf("read %s: %w", r.backup, err)
	}
	if err := safefile.Overwrite(r.path, orig); err != nil {
		return fmt.Errorf("restore %s from %s: %w; restore it with: sudo cp %s %s", r.path, r.backup, err, r.backup, r.path)
	}
	return os.Remove(r.backup)
}
