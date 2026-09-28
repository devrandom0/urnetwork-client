package main

import (
	"context"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"
)

type recordingControl struct {
	mu        sync.Mutex
	addresses []string
	err       error
}

func (r *recordingControl) control(_, address string, _ syscall.RawConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addresses = append(r.addresses, address)
	return r.err
}

func (r *recordingControl) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.addresses...)
}

func lookupWithTimeout(t *testing.T, dnsServers []string, rc *recordingControl) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := newSocksResolver(dnsServers, rc.control).LookupIP(ctx, "ip4", "example.com")
	return err
}

func TestSocksResolver_BindFailureFailsLookup(t *testing.T) {
	rc := &recordingControl{err: errBindInterface}
	if err := lookupWithTimeout(t, nil, rc); err == nil {
		t.Fatal("lookup succeeded although binding the resolver socket failed; it must not fall back to the physical interface")
	}
	if len(rc.seen()) == 0 {
		t.Fatal("system resolver servers were dialed without the bind control")
	}
}

func TestSocksResolver_CustomServerDialedThroughControl(t *testing.T) {
	rc := &recordingControl{err: errBindInterface}
	if err := lookupWithTimeout(t, []string{"192.0.2.53"}, rc); err == nil {
		t.Fatal("lookup succeeded although binding failed")
	}
	for _, a := range rc.seen() {
		if a != "192.0.2.53:53" {
			t.Fatalf("resolver dialed %q; want only the configured server", a)
		}
	}
	if len(rc.seen()) == 0 {
		t.Fatal("custom DNS server was dialed without the bind control")
	}
}

func TestSocksConnect_DomainLookupFailsClosedWhenBindFails(t *testing.T) {
	dns, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = dns.Close() }()
	queried := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 512)
		if _, _, err := dns.ReadFrom(buf); err == nil {
			queried <- struct{}{}
		}
	}()
	proxy := startSocksForTest(t, SocksOptions{BindIf: "urnet-nope0", DNSServers: []string{dns.LocalAddr().String()}})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)

	rep, _ := socksRequest(t, c, socksCmdConnect, "example.com", 443)

	if rep != socksRepHostUnreachable {
		t.Fatalf("rep = %d, want host unreachable", rep)
	}
	select {
	case <-queried:
		t.Fatal("DNS query left through an unbound socket although the tunnel bind failed")
	case <-time.After(100 * time.Millisecond):
	}
}
