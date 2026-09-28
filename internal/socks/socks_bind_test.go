package socks

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/logx"
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

// captureStderr runs fn on the calling goroutine with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	fn()
	os.Stderr = orig
	_ = w.Close()
	s := <-out
	_ = r.Close()
	return s
}

func TestSocksConnect_BindFailureWarnOmitsDestination(t *testing.T) {
	logx.SetLogLevel("info", false)
	s := &socksServer{opts: SocksOptions{BindIf: "urnet-nope0"}}
	client, proxySide := net.Pipe()
	defer func() { _ = client.Close() }()
	go func() { _, _ = io.Copy(io.Discard, client) }()

	logged := captureStderr(t, func() {
		s.handleConnect(context.Background(), proxySide, socksAddr{atyp: socksATYPIPv4, host: "198.51.100.23", port: 54321})
	})

	if !strings.Contains(logged, "refusing CONNECT") {
		t.Fatalf("stderr = %q; want the bind refusal WARN", logged)
	}
	if strings.Contains(logged, "198.51.100.23") || strings.Contains(logged, "54321") {
		t.Fatalf("stderr = %q; the WARN must not name the CONNECT destination", logged)
	}
}
