package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SocksAuth holds RFC 1929 credentials. The zero value disables authentication.
type SocksAuth struct {
	User string
	Pass string
}

func (a SocksAuth) Enabled() bool { return a.User != "" || a.Pass != "" }

func (a SocksAuth) validate() error {
	if !a.Enabled() {
		return nil
	}
	if a.User == "" || a.Pass == "" {
		return errors.New("SOCKS auth needs both a username and a password")
	}
	if len(a.User) > 255 || len(a.Pass) > 255 {
		return errors.New("SOCKS username and password must be at most 255 bytes (RFC 1929)")
	}
	return nil
}

func (a SocksAuth) matches(user, pass []byte) bool {
	// Hashing first keeps the comparison constant-time even when lengths differ.
	wantU, gotU := sha256.Sum256([]byte(a.User)), sha256.Sum256(user)
	wantP, gotP := sha256.Sum256([]byte(a.Pass)), sha256.Sum256(pass)
	return subtle.ConstantTimeCompare(wantU[:], gotU[:])&subtle.ConstantTimeCompare(wantP[:], gotP[:]) == 1
}

func selectSocksMethod(offered []byte, authRequired bool) byte {
	want := byte(socksMethodNoAuth)
	if authRequired {
		want = socksMethodUserPass
	}
	for _, m := range offered {
		if m == want {
			return want
		}
	}
	return socksMethodNoAcceptable
}

const (
	socksUserPassVersion = 0x01
	socksUserPassOK      = 0x00
	socksUserPassFailed  = 0x01
)

var errSocksAuthFailed = errors.New("invalid SOCKS username or password")

func socksUserPassAuth(rw io.ReadWriter, want SocksAuth) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(rw, head); err != nil {
		return err
	}
	if head[0] != socksUserPassVersion {
		_, _ = rw.Write([]byte{socksUserPassVersion, socksUserPassFailed})
		return fmt.Errorf("unsupported auth sub-negotiation version %d", head[0])
	}
	user := make([]byte, int(head[1]))
	if _, err := io.ReadFull(rw, user); err != nil {
		return err
	}
	plen := make([]byte, 1)
	if _, err := io.ReadFull(rw, plen); err != nil {
		return err
	}
	pass := make([]byte, int(plen[0]))
	if _, err := io.ReadFull(rw, pass); err != nil {
		return err
	}
	if !want.matches(user, pass) {
		_, _ = rw.Write([]byte{socksUserPassVersion, socksUserPassFailed})
		return errSocksAuthFailed
	}
	_, err := rw.Write([]byte{socksUserPassVersion, socksUserPassOK})
	return err
}

const (
	envSocksUser = "URNETWORK_SOCKS_USER"
	envSocksPass = "URNETWORK_SOCKS_PASS"
)

// resolveSocksAuth prefers flags and falls back to the environment so the password can
// stay off the command line, where every local user can read it.
func resolveSocksAuth(flagUser, flagPass string, getenv func(string) string) SocksAuth {
	a := SocksAuth{User: strings.TrimSpace(flagUser), Pass: flagPass}
	if a.User == "" {
		a.User = strings.TrimSpace(getenv(envSocksUser))
	}
	if a.Pass == "" {
		a.Pass = getenv(envSocksPass)
	}
	return a
}
