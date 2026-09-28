package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
)

func TestReadSocksAddr(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		want    socksAddr
		wantErr error
	}{
		{"ipv4", []byte{1, 10, 0, 0, 1, 0x01, 0xBB}, socksAddr{atyp: 1, host: "10.0.0.1", port: 443}, nil},
		{"domain", append(append([]byte{3, 11}, "example.com"...), 0, 80), socksAddr{atyp: 3, host: "example.com", port: 80}, nil},
		{"ipv6", append(append([]byte{4}, net.ParseIP("2001:db8::1").To16()...), 0x1F, 0x90), socksAddr{atyp: 4, host: "2001:db8::1", port: 8080}, nil},
		{"bad atyp", []byte{9, 0, 0}, socksAddr{atyp: 9}, errSocksBadATYP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readSocksAddr(bytes.NewReader(tc.in))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWriteSocksReply_EncodesBindAddr(t *testing.T) {
	v6 := net.ParseIP("2001:db8::2")
	cases := []struct {
		name string
		addr net.Addr
		want []byte
	}{
		{"nil", nil, []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}},
		{"tcp4", &net.TCPAddr{IP: net.IPv4(192, 0, 2, 7), Port: 1080}, []byte{5, 0, 0, 1, 192, 0, 2, 7, 0x04, 0x38}},
		{"udp6", &net.UDPAddr{IP: v6, Port: 53}, append(append([]byte{5, 0, 0, 4}, v6.To16()...), 0, 53)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeSocksReply(&buf, socksRepSucceeded, tc.addr); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf.Bytes(), tc.want) {
				t.Fatalf("got %v, want %v", buf.Bytes(), tc.want)
			}
		})
	}
}

func TestSocksConnect_AddressTypes(t *testing.T) {
	echo4 := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{})
	t.Run("ipv4", func(t *testing.T) { assertSocksEcho(t, proxy, "127.0.0.1", echo4.Port) })
	t.Run("domain", func(t *testing.T) { assertSocksEcho(t, proxy, "localhost", echo4.Port) })
	t.Run("ipv6", func(t *testing.T) {
		echo6 := startTCPEcho(t, "tcp6", "[::1]:0")
		assertSocksEcho(t, proxy, "::1", echo6.Port)
	})
}

func TestSocksRequest_UnsupportedCommand(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	const cmdBind = 0x02
	if rep, _ := socksRequest(t, c, cmdBind, "127.0.0.1", 80); rep != socksRepCmdNotSupported {
		t.Fatalf("BIND rep = %d, want %d", rep, socksRepCmdNotSupported)
	}
}

func TestSocksRequest_UnsupportedAddressType(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	if _, err := c.Write([]byte{5, socksCmdConnect, 0, 9}); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil || resp[1] != socksRepAddrTypeNotSupported {
		t.Fatalf("reply = %v, %v; want rep %d", resp, err, socksRepAddrTypeNotSupported)
	}
}
