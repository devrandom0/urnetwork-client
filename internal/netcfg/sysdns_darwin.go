//go:build darwin

package netcfg

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// scutilService is a dynamic-store service of our own. It is not in the saved network
// settings, so configd drops it at reboot and nothing else ever rewrites it.
const scutilService = "State:/Network/Service/urnet-client"

const scutilRemoveScript = "remove " + scutilService + "/DNS\nremove " + scutilService + "/IPv4\n"

// NewSystemDNS returns the macOS setter, which publishes the stub through scutil.
func NewSystemDNS(tunName string) SystemDNS {
	return &scutilDNS{tun: tunName, wait: 3 * time.Second, tick: 100 * time.Millisecond}
}

// RecoverStaleDNS removes the scutil entry left by a run that died before its cleanup; while
// it exists every lookup goes to a stub that is no longer running.
func RecoverStaleDNS() error {
	out, err := cmdRunner.RunInput("show "+scutilService+"/DNS\n", "scutil")
	if err != nil {
		return fmt.Errorf("scutil: %w", err)
	}
	if strings.Contains(out, "No such key") {
		return nil
	}
	logx.Warn("removing DNS settings left by an earlier urnet-client run\n")
	return removeScutilService()
}

type scutilDNS struct {
	tun        string
	applied    bool
	wait, tick time.Duration
}

// Apply publishes the stub as a resolver with an empty supplemental match domain, which
// matches every name and sorts ahead of the primary service's resolver. The IPv4 entry has
// no Router key, so configd never makes the service primary or touches routes.
func (s *scutilDNS) Apply(stub netip.Addr) error {
	script := strings.Join([]string{
		"d.init",
		"d.add ServerAddresses * " + stub.String(),
		`d.add SupplementalMatchDomains * ""`,
		"d.add SearchOrder # 1",
		"set " + scutilService + "/DNS",
		"d.init",
		"d.add Addresses * " + stub.String(),
		"d.add SubnetMasks * 255.255.255.255",
		"d.add InterfaceName " + s.tun,
		"set " + scutilService + "/IPv4",
		"",
	}, "\n")
	out, err := cmdRunner.RunInput(script, "scutil")
	if err == nil {
		err = scutilOutputErr(out, false)
	}
	if err != nil {
		_ = removeScutilService()
		return fmt.Errorf("scutil: publish DNS stub %s: %w", stub, err)
	}
	s.applied = true
	if err := s.waitUntilDefault(stub); err != nil {
		_ = s.Restore()
		return err
	}
	logx.Info("DNS: system resolver points at the stub %s\n", stub)
	return nil
}

func (s *scutilDNS) Restore() error {
	if !s.applied {
		return nil
	}
	s.applied = false
	return removeScutilService()
}

// waitUntilDefault fails closed when configd does not put the stub first, for example on a
// macOS version that orders resolvers differently.
func (s *scutilDNS) waitUntilDefault(stub netip.Addr) error {
	deadline := time.Now().Add(s.wait)
	var got string
	for {
		out, err := runCapture("scutil", "--dns")
		if err == nil {
			if got = defaultResolver(out); got == stub.String() {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("scutil: the system resolver is %q, not the DNS stub %s; refusing to run with DNS outside the tunnel", got, stub)
		}
		time.Sleep(s.tick)
	}
}

// defaultResolver returns nameserver[0] of resolver #1 in the unscoped section of `scutil --dns`.
func defaultResolver(out string) string {
	inFirst := false
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(ln, "DNS configuration (for scoped queries)"):
			return ""
		case ln == "resolver #1":
			inFirst = true
		case strings.HasPrefix(ln, "resolver #"):
			if inFirst {
				return ""
			}
		case inFirst && strings.HasPrefix(ln, "nameserver[0]"):
			if _, v, ok := strings.Cut(ln, ":"); ok {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func removeScutilService() error {
	out, err := cmdRunner.RunInput(scutilRemoveScript, "scutil")
	if err == nil {
		err = scutilOutputErr(out, true)
	}
	if err != nil {
		return fmt.Errorf("scutil: remove %s: %w; remove it with: printf 'remove %s/DNS\\nremove %s/IPv4\\n' | sudo scutil", scutilService, err, scutilService, scutilService)
	}
	return nil
}

// scutilOutputErr turns scutil's stdout into an error: in stdin mode it exits 0 even when a
// command fails and only prints the reason, such as "Permission denied".
func scutilOutputErr(out string, allowMissing bool) error {
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || (allowMissing && ln == "No such key") {
			continue
		}
		return errors.New(ln)
	}
	return nil
}
