//go:build darwin

package netcfg

import (
	"strings"
	"testing"
)

func TestConfigureDarwinTUN_FailsOnIfconfigError(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ifconfig utun9 inet 10.255.0.2 10.255.0.1 mtu 1420 up", "ifconfig: ioctl (SIOCAIFADDR): Operation not permitted")

	if _, err := ConfigureDarwinTUN("utun9", "10.255.0.2/24", 1420, false); err == nil {
		t.Fatal("want error when ifconfig fails")
	}
	for _, c := range f.Calls() {
		if strings.HasPrefix(c, "route") {
			t.Fatalf("route command %q ran during TUN setup", c)
		}
	}
}

func TestConfigureDarwinTUN_NoIPv6AddressWhenDisabled(t *testing.T) {
	f := useFakeRunner(t)

	peer, err := ConfigureDarwinTUN("utun9", "10.255.0.2/24", 1420, false)

	if err != nil || peer != "10.255.0.1" {
		t.Fatalf("got peer=%q err=%v; want 10.255.0.1, nil", peer, err)
	}
	for _, c := range f.Calls() {
		if strings.Contains(c, "inet6") {
			t.Fatalf("ran %q with --enable_ipv6 off; the TUN must get no IPv6 address", c)
		}
	}
}

func TestConfigureDarwinTUN_IPv6AddressRequiredWhenEnabled(t *testing.T) {
	f := useFakeRunner(t)
	if _, err := ConfigureDarwinTUN("utun9", "10.255.0.2/24", 1420, true); err != nil {
		t.Fatalf("IPv6 enabled: %v", err)
	}
	if f.count("ifconfig utun9 inet6 fd00::2/120") != 1 {
		t.Fatalf("calls = %v; want the IPv6 address assigned", f.Calls())
	}

	f.failOn("ifconfig utun9 inet6 fd00::2/120", "ifconfig: inet6: bad value")
	if _, err := ConfigureDarwinTUN("utun9", "10.255.0.2/24", 1420, true); err == nil {
		t.Fatal("IPv6 enabled: want error when the IPv6 address cannot be set")
	}
}

func TestDefaultGateway_UsesCommandRunner(t *testing.T) {
	f := useFakeRunner(t)
	f.results["route -n get default"] = fakeResult{stdout: "   route to: default\ndestination: default\n    gateway: 192.168.1.1\n  interface: en0\n"}
	gw, iface, err := DefaultGateway()
	if err != nil || gw != "192.168.1.1" || iface != "en0" {
		t.Fatalf("DefaultGateway = %q, %q, %v; want the scripted gateway via the command runner", gw, iface, err)
	}
}

func TestDefaultGateway_IgnoresStderrJunk(t *testing.T) {
	f := useFakeRunner(t)
	f.results["route -n get default"] = fakeResult{
		stdout: "   route to: default\ndestination: default\n    gateway: 192.168.1.1\n  interface: en0\n",
		// A real `route` invocation can write unrelated warnings to stderr; if DefaultGateway
		// parsed CombinedOutput this decoy line (with its own "gateway:"/"interface:" prefixes)
		// would win because the parser keeps the last match.
		out: "   route to: default\ndestination: default\n    gateway: 192.168.1.1\n  interface: en0\n" +
			"routing table warning: gateway: 6.6.6.6\n  interface: bogus0\n",
	}
	gw, iface, err := DefaultGateway()
	if err != nil || gw != "192.168.1.1" || iface != "en0" {
		t.Fatalf("DefaultGateway = %q, %q, %v; want the real gateway, not stderr junk", gw, iface, err)
	}
}
