//go:build linux

package netcfg

import "testing"

func TestParseOwnerPID(t *testing.T) {
	for in, want := range map[string]int{
		"# urnet-client pid=4242\nnameserver 10.255.0.2\n": 4242,
		"pid=77 tun=urnet0\n":                              77,
		"nameserver 10.255.0.2\n":                          0,
		"# urnet-client pid=abc\n":                         0,
	} {
		if got := parseOwnerPID(in); got != want {
			t.Fatalf("parseOwnerPID(%q) = %d, want %d", in, got, want)
		}
	}
}
