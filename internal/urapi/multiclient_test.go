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
	refuse   bool
}

func (f *fakeSender) SendPacket(_ connect.TransferPath, _ protocol.ProvideMode, _ []byte, timeout time.Duration) bool {
	f.timeouts = append(f.timeouts, timeout)
	time.Sleep(f.delay)
	return !f.refuse
}

func (f *fakeSender) Close() {}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	logx.SetLogLevel("info", false)
	return captureFile(t, &os.Stderr, fn)
}

func captureDebug(t *testing.T, fn func()) string {
	t.Helper()
	logx.SetLogLevel("debug", false)
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	return captureFile(t, &os.Stdout, fn)
}

func captureFile(t *testing.T, f **os.File, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := *f
	*f = w
	fn()
	*f = orig
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

func useSlowSendWarning(t *testing.T, d time.Duration) {
	t.Helper()
	old := slowSendWarning
	slowSendWarning = d
	t.Cleanup(func() { slowSendWarning = old })
}

// During a real outage every send is slow; one WARN per couple of seconds for hours
// buries everything else in the log.
func TestSendPacket_SlowSendWarningIsRateLimited(t *testing.T) {
	useSlowSendWarning(t, time.Millisecond)
	m := &MultiClient{mc: &fakeSender{delay: 5 * time.Millisecond}}

	logged := captureStderr(t, func() {
		for i := 0; i < 3; i++ {
			m.SendPacket([]byte{0x45})
		}
	})

	if n := strings.Count(logged, "provider window"); n != 1 {
		t.Fatalf("stderr = %q; want one WARN for three slow sends in a row", logged)
	}
}

func TestWarnLimiter_ReportsWhatItSuppressed(t *testing.T) {
	var l warnLimiter
	t0 := time.Unix(1000, 0)

	if ok, _ := l.allow(t0); !ok {
		t.Fatal("the first warning must be logged")
	}
	for i := 1; i <= 4; i++ {
		if ok, _ := l.allow(t0.Add(time.Duration(i) * time.Second)); ok {
			t.Fatalf("warning %d within %s was logged", i, slowSendWarnEvery)
		}
	}
	ok, suppressed := l.allow(t0.Add(slowSendWarnEvery))
	if !ok || suppressed != 4 {
		t.Fatalf("allow after %s = %v, %d; want logged with 4 suppressed", slowSendWarnEvery, ok, suppressed)
	}
	if _, suppressed := l.allow(t0.Add(3 * slowSendWarnEvery)); suppressed != 0 {
		t.Fatalf("suppressed = %d; the count must restart after each logged warning", suppressed)
	}
}

func TestSendPacket_SlowSendWarningCountsSuppressed(t *testing.T) {
	useSlowSendWarning(t, time.Millisecond)
	m := &MultiClient{mc: &fakeSender{delay: 5 * time.Millisecond}}
	m.slowWarn.last = time.Now().Add(-slowSendWarnEvery - time.Second)
	m.slowWarn.suppressed = 7

	logged := captureStderr(t, func() { m.SendPacket([]byte{0x45}) })

	if !strings.Contains(logged, "7 more slow sends") {
		t.Fatalf("stderr = %q; want the count of suppressed warnings", logged)
	}
}

// connect also refuses a packet it cannot parse or once it is closing; calling every
// refusal a timeout sends whoever reads the debug log after the wrong cause.
func TestSendPacket_DropReasonMatchesWhatHappened(t *testing.T) {
	logged := captureDebug(t, func() { (&MultiClient{mc: &fakeSender{refuse: true}}).SendPacket([]byte{0x45}) })

	if !strings.Contains(logged, "dropped outbound packet") || strings.Contains(logged, "within") {
		t.Fatalf("stdout = %q; a send refused at once must not be reported as a provider timeout", logged)
	}
}
