package socks

import (
	"net"
	"testing"
)

func TestSocksExposedWithoutAuth(t *testing.T) {
	auth := SocksAuth{User: "u", Pass: "p"}
	lan := &net.TCPAddr{IP: net.IPv4(192, 168, 88, 2), Port: 1080}
	cases := []struct {
		name string
		addr net.Addr
		auth SocksAuth
		want bool
	}{
		{"loopback v4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1080}, SocksAuth{}, false},
		{"loopback v6", &net.TCPAddr{IP: net.IPv6loopback, Port: 1080}, SocksAuth{}, false},
		{"all interfaces v4", &net.TCPAddr{IP: net.IPv4zero, Port: 1080}, SocksAuth{}, true},
		{"all interfaces v6", &net.TCPAddr{IP: net.IPv6unspecified, Port: 1080}, SocksAuth{}, true},
		{"lan address", lan, SocksAuth{}, true},
		{"lan address with auth", lan, auth, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := socksExposedWithoutAuth(tc.addr, tc.auth); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
