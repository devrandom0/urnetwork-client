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

func TestConfigureDarwinTUN_ReturnsPeerAndToleratesIPv6WhenDisabled(t *testing.T) {
	f := useFakeRunner(t)
	f.failOn("ifconfig utun9 inet6 fd00::2/120", "ifconfig: inet6: bad value")

	peer, err := ConfigureDarwinTUN("utun9", "10.255.0.2/24", 1420, false)
	if err != nil || peer != "10.255.0.1" {
		t.Fatalf("got peer=%q err=%v; want 10.255.0.1, nil", peer, err)
	}
	if _, err := ConfigureDarwinTUN("utun9", "10.255.0.2/24", 1420, true); err == nil {
		t.Fatal("IPv6 enabled: want error")
	}
}
