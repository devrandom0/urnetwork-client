//go:build linux

package netcfg

import (
	"strings"
	"testing"
)

func TestConfigureLinuxTUN_FailsFastOnAddressError(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ip addr add 10.255.0.2/24 dev urnet0", "RTNETLINK answers: Operation not permitted")

	err := ConfigureLinuxTUN("urnet0", "10.255.0.2/24", 1420, false)

	if err == nil {
		t.Fatal("want error when the TUN address cannot be set")
	}
	for _, c := range f.Calls() {
		if strings.Contains(c, " up") || strings.HasPrefix(c, "ip route") {
			t.Fatalf("ran %q after the address failed; setup must stop at the first error", c)
		}
	}
}

func TestConfigureLinuxTUN_NoIPv6AddressWhenDisabled(t *testing.T) {
	f := useFakeRunner(t)

	if err := ConfigureLinuxTUN("urnet0", "10.255.0.2/24", 1420, false); err != nil {
		t.Fatalf("IPv6 disabled: %v", err)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c, "fd00::") {
			t.Fatalf("ran %q with --enable_ipv6 off; the TUN must get no IPv6 address", c)
		}
	}
}

func TestConfigureLinuxTUN_IPv6AddressRequiredWhenEnabled(t *testing.T) {
	f := useFakeRunner(t)
	if err := ConfigureLinuxTUN("urnet0", "10.255.0.2/24", 1420, true); err != nil {
		t.Fatalf("IPv6 enabled: %v", err)
	}
	if f.count("ip addr add fd00::2/120 dev urnet0") != 1 {
		t.Fatalf("calls = %v; want the IPv6 address assigned", f.Calls())
	}

	f.failOn("ip addr add fd00::2/120 dev urnet0", "RTNETLINK answers: Permission denied")
	if err := ConfigureLinuxTUN("urnet0", "10.255.0.2/24", 1420, true); err == nil {
		t.Fatal("IPv6 enabled: want error when the IPv6 address cannot be set")
	}
}
