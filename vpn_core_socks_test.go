package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunSocksOnly_NoListenIsError(t *testing.T) {
	err := runSocksOnly(context.Background(), VPNConfig{})
	if err == nil || !strings.Contains(err.Error(), "nothing to do") {
		t.Fatalf("want a 'nothing to do' error so the process exits non-zero, got %v", err)
	}
}

func TestRunSocksOnly_ServesUntilCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runSocksOnly(ctx, VPNConfig{SOCKSListen: "127.0.0.1:0"}) }()
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
	cfg := VPNConfig{
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
