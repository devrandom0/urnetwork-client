package main

import (
	"net"
	"testing"
)

// buildTCPPacket constructs a minimal IPv4/TCP packet with the given flags byte.
// src is the source IPv4 address; flags contains the TCP flag bits (e.g. 0x02 = SYN).
func buildTCPPacket(src net.IP, flags byte) []byte {
	src4 := src.To4()
	if src4 == nil {
		panic("buildTCPPacket: non-IPv4 address")
	}
	pkt := make([]byte, 40) // 20-byte IP header + 20-byte TCP header
	// IP header
	pkt[0] = 0x45 // version=4, IHL=5 (5*4=20 bytes)
	pkt[9] = 6    // protocol: TCP
	// source IP
	copy(pkt[12:16], src4)
	// destination IP (arbitrary)
	copy(pkt[16:20], []byte{10, 0, 0, 1})
	// TCP flags at offset ihl+13 = 20+13 = 33
	pkt[33] = flags
	return pkt
}

func TestShouldDropInbound(t *testing.T) {
	_, allow192, _ := net.ParseCIDR("192.168.0.0/16")
	_, allow10, _ := net.ParseCIDR("10.0.0.0/8")

	cases := []struct {
		name      string
		srcIP     string
		flags     byte
		allowList []*net.IPNet
		wantDrop  bool
	}{
		// SYN only (new inbound connection)
		{"SYN from allowed CIDR", "192.168.1.1", 0x02, []*net.IPNet{allow192}, false},
		{"SYN from different allowed CIDR", "10.1.2.3", 0x02, []*net.IPNet{allow10}, false},
		{"SYN from multiple CIDRs — matches second", "10.1.2.3", 0x02, []*net.IPNet{allow192, allow10}, false},
		{"SYN not in any CIDR — drop", "172.16.0.1", 0x02, []*net.IPNet{allow192, allow10}, true},
		{"SYN with empty allowList — always drop", "10.0.0.1", 0x02, []*net.IPNet{}, true},
		{"SYN with nil allowList — always drop", "10.0.0.1", 0x02, nil, true},

		// SYN-ACK (response; should not be dropped)
		{"SYN-ACK from any IP — pass", "198.51.100.1", 0x12, []*net.IPNet{allow192}, false},

		// ACK only (established connection)
		{"ACK only — pass", "172.16.0.1", 0x10, []*net.IPNet{}, false},

		// RST without ACK — drop
		{"RST no ACK — drop", "1.2.3.4", 0x04, []*net.IPNet{allow192}, true},
		// RST+ACK — pass (ack bit set)
		{"RST+ACK — pass", "1.2.3.4", 0x14, []*net.IPNet{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := buildTCPPacket(net.ParseIP(tc.srcIP), tc.flags)
			got := shouldDropInbound(pkt, tc.allowList)
			if got != tc.wantDrop {
				t.Errorf("shouldDropInbound(%s, flags=0x%02x, %d CIDRs) = %v; want %v",
					tc.srcIP, tc.flags, len(tc.allowList), got, tc.wantDrop)
			}
		})
	}
}

func TestShouldDropInbound_NonTCP(t *testing.T) {
	// UDP packet — should never be dropped by this function
	pkt := make([]byte, 40)
	pkt[0] = 0x45
	pkt[9] = 17 // UDP
	got := shouldDropInbound(pkt, nil)
	if got {
		t.Error("shouldDropInbound should return false for UDP packets")
	}
}

func TestShouldDropInbound_TooShort(t *testing.T) {
	// Packets too short to parse should pass through
	cases := []struct {
		name string
		pkt  []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"19 bytes (< 20)", make([]byte, 19)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldDropInbound(tc.pkt, nil)
			if got {
				t.Errorf("shouldDropInbound(%q) = true; want false for short packet", tc.name)
			}
		})
	}
}

func TestParseCIDRHost(t *testing.T) {
	cases := []struct {
		input   string
		wantNil bool
		wantIP  string
		wantLen int // prefix length
	}{
		{"", true, "", 0},
		{"   ", true, "", 0},
		// Host addresses
		{"10.0.0.1", false, "10.0.0.1", 32},
		{"192.168.1.100", false, "192.168.1.100", 32},
		// CIDR notation
		{"10.0.0.0/8", false, "10.0.0.0", 8},
		{"192.168.1.0/24", false, "192.168.1.0", 24},
		{"10.1.2.3/16", false, "10.1.2.3", 16}, // host IP preserved (not masked)
		// Invalid inputs
		{"not-an-ip", true, "", 0},
		{"10.0.0.900", true, "", 0},
		{"10.0.0.1/33", true, "", 0},
		// IPv6 — not supported
		{"::1", true, "", 0},
		{"2001:db8::/32", true, "", 0},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseCIDRHost(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Errorf("parseCIDRHost(%q) = %v; want nil", tc.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseCIDRHost(%q) = nil; want non-nil", tc.input)
			}
			ones, _ := got.Mask.Size()
			if ones != tc.wantLen {
				t.Errorf("parseCIDRHost(%q) prefix len = %d; want %d", tc.input, ones, tc.wantLen)
			}
			wantIP := net.ParseIP(tc.wantIP).To4()
			if !got.IP.Equal(wantIP) {
				t.Errorf("parseCIDRHost(%q).IP = %v; want %v", tc.input, got.IP, wantIP)
			}
		})
	}
}
