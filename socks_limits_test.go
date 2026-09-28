package main

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestSocks_ConnCapClosesConnectionsOverTheLimit(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{MaxConns: 2})
	held := []net.Conn{dialSocks(t, proxy), dialSocks(t, proxy)}
	for _, c := range held {
		socksGreet(t, c, socksMethodNoAuth)
	}

	extra := dialSocks(t, proxy)
	_ = extra.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _ = extra.Write([]byte{5, 1, socksMethodNoAuth})
	var ne net.Error
	if n, err := io.ReadFull(extra, make([]byte, 2)); err == nil || (errors.As(err, &ne) && ne.Timeout()) {
		t.Fatalf("read over the cap = %d bytes, %v; want the connection closed (EOF or reset) without a handshake", n, err)
	}

	_ = held[0].Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", proxy, time.Second)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		_ = c.SetDeadline(time.Now().Add(time.Second))
		_, _ = c.Write([]byte{5, 1, socksMethodNoAuth})
		resp := make([]byte, 2)
		_, err = io.ReadFull(c, resp)
		_ = c.Close()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("slot was not released after a held connection closed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSocksUDPAssociate_IdleTimeoutEndsAssociation(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{UDPIdleTimeout: 200 * time.Millisecond})
	ctrl, relay := udpAssociate(t, proxy)
	client := newUDPClient(t, "127.0.0.1:0")
	if err := udpRoundTrip(t, client, relay, echo.Port, "ping"); err != nil {
		t.Fatalf("association not working: %v", err)
	}

	_ = ctrl.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := ctrl.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("control read = %v; want the idle association closed by the server", err)
	}
	if err := udpRoundTrip(t, client, relay, echo.Port, "late"); err == nil {
		t.Fatal("relay still forwarding after the idle timeout")
	}
}

func TestSocksUDPAssociate_ActivityKeepsAssociationAlive(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{UDPIdleTimeout: 300 * time.Millisecond})
	_, relay := udpAssociate(t, proxy)
	client := newUDPClient(t, "127.0.0.1:0")
	for i := 0; i < 6; i++ {
		if err := udpRoundTrip(t, client, relay, echo.Port, "ping"); err != nil {
			t.Fatalf("round trip %d: %v; traffic must reset the idle timer", i, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestSocksUserPass_FailureIsDelayed(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{Auth: SocksAuth{User: "alice", Pass: "s3cret"}})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodUserPass)
	start := time.Now()
	if st := socksUserPass(t, c, "alice", "wrong"); st == 0 {
		t.Fatal("wrong password accepted")
	}
	if took := time.Since(start); took < socksAuthFailureDelay-50*time.Millisecond {
		t.Fatalf("failed auth answered after %s; want at least %s to slow down guessing", took, socksAuthFailureDelay)
	}
}

func TestLogRateLimiter_OneLinePerKeyPerInterval(t *testing.T) {
	l := newLogRateLimiter(time.Minute)
	t0 := time.Unix(1_000_000, 0)
	if !l.allow("192.0.2.1", t0) {
		t.Fatal("first event must be logged")
	}
	if l.allow("192.0.2.1", t0.Add(30*time.Second)) {
		t.Fatal("second event within the interval must be suppressed")
	}
	if !l.allow("192.0.2.2", t0.Add(30*time.Second)) {
		t.Fatal("another source must not be suppressed")
	}
	if !l.allow("192.0.2.1", t0.Add(61*time.Second)) {
		t.Fatal("event after the interval must be logged again")
	}
}

func TestLogRateLimiter_PrunesStaleKeys(t *testing.T) {
	l := newLogRateLimiter(time.Minute)
	t0 := time.Unix(1_000_000, 0)
	for i := 0; i < maxLogRateLimiterKeys+10; i++ {
		l.allow(net.IPv4(10, 0, byte(i>>8), byte(i)).String(), t0)
	}
	l.allow("192.0.2.9", t0.Add(2*time.Minute))
	if n := l.size(); n > maxLogRateLimiterKeys {
		t.Fatalf("limiter holds %d keys; stale keys must be pruned", n)
	}
}
