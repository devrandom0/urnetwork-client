package urapi

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/connect/protocol"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

type fakeSender struct {
	timeouts []time.Duration
	delay    time.Duration
}

func (f *fakeSender) SendPacket(_ connect.TransferPath, _ protocol.ProvideMode, _ []byte, timeout time.Duration) bool {
	f.timeouts = append(f.timeouts, timeout)
	time.Sleep(f.delay)
	return true
}

func (f *fakeSender) Close() {}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	logx.SetLogLevel("info", false)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = orig
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// A negative timeout makes connect wait forever for a provider window, which wedges the
// TUN reader on the first packet sent before any provider is up.
func TestSendPacket_WaitIsBounded(t *testing.T) {
	f := &fakeSender{}
	(&MultiClient{mc: f}).SendPacket([]byte{0x45})

	if len(f.timeouts) != 1 || f.timeouts[0] <= 0 || f.timeouts[0] > 5*time.Second {
		t.Fatalf("send timeouts = %v; want one bounded, positive wait", f.timeouts)
	}
}

func TestSendPacket_WarnsWhenASendBlocks(t *testing.T) {
	old := slowSendWarning
	slowSendWarning = 10 * time.Millisecond
	t.Cleanup(func() { slowSendWarning = old })
	f := &fakeSender{delay: 30 * time.Millisecond}

	logged := captureStderr(t, func() { (&MultiClient{mc: f}).SendPacket([]byte{0x45}) })

	if !strings.Contains(logged, "provider window") {
		t.Fatalf("stderr = %q; want a WARN that the send waited for a provider window", logged)
	}
}

func TestSendPacket_FastSendIsQuiet(t *testing.T) {
	logged := captureStderr(t, func() { (&MultiClient{mc: &fakeSender{}}).SendPacket([]byte{0x45}) })
	if logged != "" {
		t.Fatalf("stderr = %q; want nothing for a normal send", logged)
	}
}
