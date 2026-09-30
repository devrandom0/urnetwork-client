//go:build darwin

package netcfg

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

const scutilDNSWithStub = `
DNS configuration

resolver #1
  search domain[0] : lan
  nameserver[0] : 10.255.0.2
  flags    : Request A records
  reach    : 0x00000003 (Reachable,Transient Connection)
  order    : 1

resolver #2
  nameserver[0] : 192.168.1.1
  if_index : 15 (en0)

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 192.168.1.1
  if_index : 15 (en0)
`

const scutilDNSWithoutStub = `
DNS configuration

resolver #1
  nameserver[0] : 192.168.1.1
  if_index : 15 (en0)

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 10.255.0.2
`

func testScutilDNS() *scutilDNS {
	return &scutilDNS{tun: "utun9", wait: 30 * time.Millisecond, tick: 5 * time.Millisecond}
}

func TestScutilDNS_ApplyPublishesStubFirst(t *testing.T) {
	f := useFakeRunner(t)
	f.results["scutil --dns"] = fakeResult{out: scutilDNSWithStub}

	if err := testScutilDNS().Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	in := f.Inputs()
	if len(in) != 1 {
		t.Fatalf("scutil stdin calls = %d, want 1", len(in))
	}
	for _, line := range []string{
		"d.add ServerAddresses * 10.255.0.2",
		`d.add SupplementalMatchDomains * ""`,
		"set State:/Network/Service/urnet-client/DNS",
		"d.add InterfaceName utun9",
		"set State:/Network/Service/urnet-client/IPv4",
	} {
		if !strings.Contains(in[0], line+"\n") {
			t.Fatalf("scutil script = %q; missing %q", in[0], line)
		}
	}
	if strings.Contains(in[0], "Router") || strings.Contains(in[0], "OverridePrimary") {
		t.Fatal("the script must not make configd treat the tunnel as the primary service")
	}
}

func TestScutilDNS_ApplyFailsWhenScutilPrintsError(t *testing.T) {
	f := useFakeRunner(t)
	f.results["scutil"] = fakeResult{out: "  Permission denied\n"}

	if err := testScutilDNS().Apply(netip.MustParseAddr("10.255.0.2")); err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("Apply = %v; scutil exits 0 on errors, so its output must be checked", err)
	}
}

func TestScutilDNS_ApplyFailsClosedWhenStubIsNotDefault(t *testing.T) {
	f := useFakeRunner(t)
	f.results["scutil --dns"] = fakeResult{out: scutilDNSWithoutStub}

	err := testScutilDNS().Apply(netip.MustParseAddr("10.255.0.2"))

	if err == nil || !strings.Contains(err.Error(), "192.168.1.1") {
		t.Fatalf("Apply = %v; want an error naming the resolver that is still in charge", err)
	}
	in := f.Inputs()
	if len(in) != 2 || in[1] != scutilRemoveScript {
		t.Fatalf("scutil inputs = %q; want the entry removed after the check failed", in)
	}
}

func TestScutilDNS_RestoreRemovesEntryOnce(t *testing.T) {
	f := useFakeRunner(t)
	f.results["scutil --dns"] = fakeResult{out: scutilDNSWithStub}
	s := testScutilDNS()
	if err := s.Apply(netip.MustParseAddr("10.255.0.2")); err != nil {
		t.Fatal(err)
	}

	if err := s.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := s.Restore(); err != nil {
		t.Fatalf("second Restore: %v", err)
	}

	if in := f.Inputs(); len(in) != 2 || in[1] != scutilRemoveScript {
		t.Fatalf("scutil inputs = %q; want exactly one removal", in)
	}
}

func TestRecoverStaleDNS_RemovesLeftoverEntry(t *testing.T) {
	f := useFakeRunner(t)
	f.results["scutil"] = fakeResult{out: "<dictionary> {\n  ServerAddresses : <array> {\n    0 : 10.255.0.2\n  }\n}\n"}

	_ = RecoverStaleDNS()

	if in := f.Inputs(); len(in) != 2 || in[1] != scutilRemoveScript {
		t.Fatalf("scutil inputs = %q; want the stale entry removed", in)
	}
}

func TestRecoverStaleDNS_NoopWithoutEntry(t *testing.T) {
	f := useFakeRunner(t)
	f.results["scutil"] = fakeResult{out: "  No such key\n"}

	if err := RecoverStaleDNS(); err != nil {
		t.Fatalf("RecoverStaleDNS = %v", err)
	}
	if in := f.Inputs(); len(in) != 1 {
		t.Fatalf("scutil inputs = %q; want only the lookup", in)
	}
}

func TestDefaultResolver(t *testing.T) {
	if got := defaultResolver(scutilDNSWithStub); got != "10.255.0.2" {
		t.Fatalf("defaultResolver = %q, want 10.255.0.2", got)
	}
	if got := defaultResolver(scutilDNSWithoutStub); got != "192.168.1.1" {
		t.Fatalf("defaultResolver = %q; the scoped section must be ignored", got)
	}
	if got := defaultResolver("DNS configuration\n\nresolver #1\n  domain : local\nresolver #2\n  nameserver[0] : 10.0.0.1\n"); got != "" {
		t.Fatalf("defaultResolver = %q; want empty when resolver #1 has no nameserver", got)
	}
}
