//go:build darwin

package netcfg

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// scutilService is a dynamic-store service of our own. It is not in the saved network
// settings, so configd drops it at reboot and nothing else ever rewrites it.
const scutilService = "State:/Network/Service/urnet-client"

const scutilRemoveScript = "remove " + scutilService + "/DNS\nremove " + scutilService + "/IPv4\n"

// NewSystemDNS returns the macOS setter, which publishes the loopback stub through scutil.
// The entry names lo0, not the TUN: mDNSResponder sends a resolver's queries out of the
// interface its service names, so a TUN entry would push lookups into the tunnel, which
// itself needs DNS to reach its providers.
func NewSystemDNS(string) SystemDNS {
	return &scutilDNS{wait: 3 * time.Second, tick: 100 * time.Millisecond}
}

// scutilOwnerKey holds the pid of the process that published the entry.
const scutilOwnerKey = "UrnetPID"

// RecoverStaleDNS removes the scutil entry left by a run that died before its cleanup; while
// it exists every lookup goes to a stub that is no longer running. It refuses while the
// process that published the entry is still alive.
func RecoverStaleDNS() error {
	markers, err := dnsMarkers()
	if err != nil {
		return err
	}
	actions, err := recoverState(RecoverOptions{}, markers)
	for _, a := range actions {
		logx.Warn("%s, left by an earlier urnet-client run\n", a)
	}
	return err
}

// dnsMarkers returns the scutil entry, if one is published.
func dnsMarkers() ([]dnsMarker, error) {
	out, err := cmdRunner.RunInput("show "+scutilService+"/DNS\n", "scutil")
	if err != nil {
		return nil, fmt.Errorf("scutil: %w", err)
	}
	if strings.Contains(out, "No such key") {
		return nil, nil
	}
	return []dnsMarker{{pid: scutilOwnerPID(out), undo: func() (string, error) {
		if err := removeScutilService(); err != nil {
			return "", err
		}
		return "removed the DNS entry " + scutilService, nil
	}}}, nil
}

type scutilDNS struct {
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
		"d.add " + scutilOwnerKey + " # " + strconv.Itoa(os.Getpid()),
		"set " + scutilService + "/DNS",
		"d.init",
		"d.add Addresses * " + stub.String(),
		"d.add SubnetMasks * 255.255.255.255",
		"d.add InterfaceName lo0",
		"set " + scutilService + "/IPv4",
		"",
	}, "\n")
	out, err := cmdRunner.RunInput(script, "scutil")
	if err == nil {
		err = scutilOutputErr(out, false)
	}
	if err != nil {
		return errors.Join(fmt.Errorf("scutil: publish DNS stub %s: %w", stub, err), removeScutilService())
	}
	s.applied = true
	if err := s.waitUntilDefault(stub); err != nil {
		return errors.Join(err, s.Restore())
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

// scutilOwnerPID reads the "UrnetPID : <n>" line of `scutil show`; 0 when absent.
func scutilOwnerPID(show string) int {
	for _, ln := range strings.Split(show, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(ln), ":")
		if ok && strings.TrimSpace(k) == scutilOwnerKey {
			if pid, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return pid
			}
		}
	}
	return 0
}

func removeScutilService() error {
	out, err := cmdRunner.RunInput(scutilRemoveScript, "scutil")
	if err == nil {
		err = scutilOutputErr(out, true)
	}
	if err != nil {
		return fmt.Errorf("scutil: remove %s: %w; %s", scutilService, err, RecoverHint)
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
