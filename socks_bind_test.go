package main

import (
	"errors"
	"syscall"
	"testing"
)

func TestBindFDToInterface_MissingInterfaceIsError(t *testing.T) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	defer func() { _ = syscall.Close(fd) }()
	if err := bindFDToInterface(fd, "tcp4", "urnet-nope0"); !errors.Is(err, errBindInterface) {
		t.Fatalf("err = %v, want errBindInterface", err)
	}
}

func TestBindFDToInterface_EmptyNameIsNoop(t *testing.T) {
	if err := bindFDToInterface(-1, "tcp4", ""); err != nil {
		t.Fatalf("empty interface name must be a no-op, got %v", err)
	}
}

func TestSocksConnect_BindFailureRefusesInsteadOfLeaking(t *testing.T) {
	echo := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{BindIf: "urnet-nope0"})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	if rep, _ := socksRequest(t, c, socksCmdConnect, "127.0.0.1", echo.Port); rep != socksRepGeneralFailure {
		t.Fatalf("rep = %d, want general failure; the connection would have bypassed the VPN", rep)
	}
}

func TestSocksConnect_ExcludedDomainIgnoresBindIf(t *testing.T) {
	echo := startTCPEcho(t, "tcp4", "127.0.0.1:0")
	proxy := startSocksForTest(t, SocksOptions{BindIf: "urnet-nope0", ExcludeDomains: []string{"localhost"}})
	assertSocksEcho(t, proxy, "localhost", echo.Port)
}
