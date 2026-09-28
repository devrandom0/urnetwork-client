package main

import (
	"bytes"
	"net"
	"runtime"
	"testing"
	"time"
)

func startUDPEcho(t *testing.T) *net.UDPAddr {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp echo listen: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	return pc.LocalAddr().(*net.UDPAddr)
}

func socksUDPDatagram(host string, port int, payload []byte) []byte {
	return append(append([]byte{0, 0, 0}, encodeSocksAddr(host, port)...), payload...)
}

func udpAssociate(t *testing.T, proxy string) (net.Conn, *net.UDPAddr) {
	t.Helper()
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	rep, bnd := socksRequest(t, c, socksCmdUDPAssociate, "0.0.0.0", 0)
	if rep != socksRepSucceeded {
		t.Fatalf("UDP ASSOCIATE rep = %d", rep)
	}
	_ = c.SetDeadline(time.Time{})
	return c, &net.UDPAddr{IP: net.ParseIP(bnd.host), Port: bnd.port}
}

func newUDPClient(t *testing.T, addr string) net.PacketConn {
	t.Helper()
	pc, err := net.ListenPacket("udp4", addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	return pc
}

func udpRoundTrip(t *testing.T, client net.PacketConn, relay *net.UDPAddr, echoPort int, msg string) error {
	t.Helper()
	if _, err := client.WriteTo(socksUDPDatagram("127.0.0.1", echoPort, []byte(msg)), relay); err != nil {
		return err
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 2048)
	n, _, err := client.ReadFrom(buf)
	if err != nil {
		return err
	}
	from, err := readSocksAddr(bytes.NewReader(buf[3:n]))
	if err != nil || from.port != echoPort {
		t.Fatalf("reply header = %+v, %v", from, err)
	}
	if !bytes.HasSuffix(buf[:n], []byte(msg)) {
		t.Fatalf("reply payload = %q, want suffix %q", buf[:n], msg)
	}
	return nil
}

func TestSocksUDPAssociate_RoundTripOutlivesHandshakeTimeout(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{HandshakeTimeout: 150 * time.Millisecond})
	_, relay := udpAssociate(t, proxy)
	time.Sleep(400 * time.Millisecond)

	client := newUDPClient(t, "127.0.0.1:0")
	if err := udpRoundTrip(t, client, relay, echo.Port, "ping"); err != nil {
		t.Fatalf("no reply through relay after the handshake timeout: %v", err)
	}
}

func TestSocksUDPAssociate_TornDownWhenControlCloses(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{})
	base := runtime.NumGoroutine()
	ctrl, relay := udpAssociate(t, proxy)
	client := newUDPClient(t, "127.0.0.1:0")
	if err := udpRoundTrip(t, client, relay, echo.Port, "ping"); err != nil {
		t.Fatalf("association not working: %v", err)
	}

	_ = ctrl.Close()
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > base {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines leaked: before=%d after=%d", base, runtime.NumGoroutine())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := udpRoundTrip(t, client, relay, echo.Port, "late"); err == nil {
		t.Fatal("relay still forwarding after the control connection closed")
	}
}

func TestSocksUDPAssociate_DropsDatagramsFromOtherHosts(t *testing.T) {
	echo := startUDPEcho(t)
	proxy := startSocksForTest(t, SocksOptions{})
	_, relay := udpAssociate(t, proxy)
	foreign := newUDPClient(t, "127.0.0.2:0")
	if err := udpRoundTrip(t, foreign, relay, echo.Port, "spoof"); err == nil {
		t.Fatal("relay forwarded a datagram from a host other than the SOCKS client")
	}
}

func TestSocksUDPAssociate_BindFailureRefuses(t *testing.T) {
	proxy := startSocksForTest(t, SocksOptions{BindIf: "urnet-nope0"})
	c := dialSocks(t, proxy)
	socksGreet(t, c, socksMethodNoAuth)
	if rep, _ := socksRequest(t, c, socksCmdUDPAssociate, "0.0.0.0", 0); rep != socksRepGeneralFailure {
		t.Fatalf("rep = %d, want general failure instead of an unbound UDP path", rep)
	}
}
