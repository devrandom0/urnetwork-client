package socks

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
)

func TestSelectSocksMethod(t *testing.T) {
	cases := []struct {
		name    string
		offered []byte
		auth    bool
		want    byte
	}{
		{"no-auth offered, auth off", []byte{0x00}, false, 0x00},
		{"only user/pass offered, auth off", []byte{0x02}, false, 0xFF},
		{"nothing offered", nil, false, 0xFF},
		{"gssapi only", []byte{0x01}, false, 0xFF},
		{"both offered, auth on", []byte{0x00, 0x02}, true, 0x02},
		{"only no-auth offered, auth on", []byte{0x00}, true, 0xFF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectSocksMethod(tc.offered, tc.auth); got != tc.want {
				t.Fatalf("got %#x, want %#x", got, tc.want)
			}
		})
	}
}

func TestSocksAuthValidate(t *testing.T) {
	long := strings.Repeat("u", 256)
	cases := []struct {
		name string
		a    SocksAuth
		ok   bool
	}{
		{"disabled", SocksAuth{}, true},
		{"complete", SocksAuth{User: "u", Pass: "p"}, true},
		{"user only", SocksAuth{User: "u"}, false},
		{"pass only", SocksAuth{Pass: "p"}, false},
		{"user too long", SocksAuth{User: long, Pass: "p"}, false},
		{"pass too long", SocksAuth{User: "u", Pass: long}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.a.Validate(); (err == nil) != tc.ok {
				t.Fatalf("Validate() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func socksUserPass(t *testing.T, c net.Conn, user, pass string) byte {
	t.Helper()
	msg := append([]byte{1, byte(len(user))}, user...)
	msg = append(append(msg, byte(len(pass))), pass...)
	if _, err := c.Write(msg); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("read auth status: %v", err)
	}
	return resp[1]
}

func TestSocksHandshake_NoAcceptableMethodClosesConnection(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{})
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, 0x02); m != 0xFF {
		t.Fatalf("method = %#x, want 0xFF", m)
	}
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection still open after 0xFF")
	}
}

func TestSocksUserPass_SuccessThenConnect(t *testing.T) {
	echo := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{Auth: SocksAuth{User: "alice", Pass: "s3cret"}})
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, socksMethodNoAuth, socksMethodUserPass); m != socksMethodUserPass {
		t.Fatalf("method = %#x, want user/pass", m)
	}
	if st := socksUserPass(t, c, "alice", "s3cret"); st != 0 {
		t.Fatalf("auth status = %d, want 0", st)
	}
	if rep, _ := socksRequest(t, c, socksCmdConnect, "127.0.0.1", echo.Port); rep != socksRepSucceeded {
		t.Fatalf("CONNECT after auth rep = %d", rep)
	}
}

func TestSocksUserPass_WrongPasswordRejected(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{Auth: SocksAuth{User: "alice", Pass: "s3cret"}})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodUserPass)
	if st := socksUserPass(t, c, "alice", "wrong"); st == 0 {
		t.Fatal("wrong password accepted")
	}
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection still open after failed auth")
	}
}

func TestSocksUserPass_NoAuthClientRejected(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{Auth: SocksAuth{User: "alice", Pass: "s3cret"}})
	c := dialSocks(t, proxy)
	if m := socksGreet(t, c, socksMethodNoAuth); m != 0xFF {
		t.Fatalf("method = %#x, want 0xFF when auth is required", m)
	}
}

func TestStartSocks5_RejectsHalfConfiguredAuth(t *testing.T) {
	_, err := StartSocks5(context.Background(), SocksOptions{ListenAddr: "127.0.0.1:0", Auth: SocksAuth{User: "alice"}})
	if err == nil {
		t.Fatal("want error for a username without a password")
	}
}

func TestResolveSocksAuth(t *testing.T) {
	env := map[string]string{envSocksUser: "envuser", envSocksPass: "envpass"}
	withEnv := func(k string) string { return env[k] }
	noEnv := func(string) string { return "" }
	cases := []struct {
		name, flagUser, flagPass string
		getenv                   func(string) string
		want                     SocksAuth
	}{
		{"flags win", "cli", "clipass", withEnv, SocksAuth{User: "cli", Pass: "clipass"}},
		{"env fallback", "", "", withEnv, SocksAuth{User: "envuser", Pass: "envpass"}},
		{"mixed", "cli", "", withEnv, SocksAuth{User: "cli", Pass: "envpass"}},
		{"nothing set", "", "", noEnv, SocksAuth{}},
		{"password keeps spaces", "u", " p w ", noEnv, SocksAuth{User: "u", Pass: " p w "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveSocksAuth(tc.flagUser, tc.flagPass, tc.getenv); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
