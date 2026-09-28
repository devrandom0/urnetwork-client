package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/socks"
)

func TestSocksOptionsFromVPN_KeepsAuth(t *testing.T) {
	if got := socksOptionsFromVPN(config.VPNConfig{SOCKSAuth: socks.SocksAuth{User: "a", Pass: "b"}}, "").Auth; got.User != "a" {
		t.Fatalf("socksOptionsFromVPN dropped auth: %+v", got)
	}
}

func TestRunSocksOnly_NoListenIsError(t *testing.T) {
	err := runSocksOnly(context.Background(), config.VPNConfig{})
	if err == nil || !strings.Contains(err.Error(), "nothing to do") {
		t.Fatalf("want a 'nothing to do' error so the process exits non-zero, got %v", err)
	}
}

func TestRunSocksOnly_ServesUntilCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runSocksOnly(ctx, config.VPNConfig{SOCKSListen: "127.0.0.1:0"}) }()
	select {
	case err := <-done:
		t.Fatalf("returned before cancel: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runSocksOnly after cancel: %v", err)
	}
}

func TestSocksOptionsFromVPN(t *testing.T) {
	cfg := config.VPNConfig{
		SOCKSListen:    "127.0.0.1:1080",
		AllowDomains:   []string{"a.example"},
		ExcludeDomains: []string{"b.example"},
		DNSList:        "1.1.1.1, 9.9.9.9",
		Debug:          true,
	}
	got := socksOptionsFromVPN(cfg, "utun9")
	if got.ListenAddr != "127.0.0.1:1080" || got.BindIf != "utun9" || !got.Debug {
		t.Fatalf("unexpected options: %+v", got)
	}
	if len(got.DNSServers) != 2 || got.DNSServers[1] != "9.9.9.9" {
		t.Fatalf("dns servers not split: %v", got.DNSServers)
	}
	if got.AllowDomains[0] != "a.example" || got.ExcludeDomains[0] != "b.example" {
		t.Fatalf("domains not copied: %+v", got)
	}
}

func TestWarnIfSocksDNSUnset_WarnsWhenBoundWithoutDNS(t *testing.T) {
	logx.SetLogLevel("info", false)
	logged := captureStderr(t, func() {
		warnIfSocksDNSUnset("utun9", "")
	})
	if !strings.Contains(logged, "fail closed") {
		t.Fatalf("stderr = %q; want the SOCKS DNS warning", logged)
	}
}

func TestWarnIfSocksDNSUnset_SilentWithDNSConfigured(t *testing.T) {
	logx.SetLogLevel("info", false)
	logged := captureStderr(t, func() {
		warnIfSocksDNSUnset("utun9", "1.1.1.1")
	})
	if logged != "" {
		t.Fatalf("stderr = %q; want no warning when --dns is set", logged)
	}
}

func TestWarnIfSocksDNSUnset_SilentWhenNotBoundToVPNInterface(t *testing.T) {
	logx.SetLogLevel("info", false)
	logged := captureStderr(t, func() {
		warnIfSocksDNSUnset("", "")
	})
	if logged != "" {
		t.Fatalf("stderr = %q; want no warning when not bound to a VPN interface", logged)
	}
}

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
