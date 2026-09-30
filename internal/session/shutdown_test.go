package session

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func recordStep(l *leakLog, name string, d time.Duration) func() error {
	return func() error {
		l.add("start " + name)
		time.Sleep(d)
		l.add("done " + name)
		return nil
	}
}

func blockForever(l *leakLog, name string) func() error {
	return func() error {
		l.add("start " + name)
		select {}
	}
}

func TestShutdown_RestoresDNSFirstThenUnwindsInReverse(t *testing.T) {
	l := &leakLog{}
	sd := newShutdown()
	sd.add("close TUN", time.Second, recordStep(l, "tun", 0))
	sd.add("remove routes", time.Second, recordStep(l, "routes", 0))
	sd.restoreDNSFirst("restore system DNS", time.Second, recordStep(l, "dns", 0))
	sd.add("close connect client", time.Second, recordStep(l, "client", 0))

	if !sd.run(nil) {
		t.Fatal("run = false; DNS was restored")
	}

	want := []string{"start dns", "done dns", "start client", "done client", "start routes", "done routes", "start tun", "done tun"}
	if got := l.all(); !slices.Equal(got, want) {
		t.Fatalf("steps = %v; want %v", got, want)
	}
}

func TestShutdown_BlockedStepsCannotHangIt(t *testing.T) {
	l := &leakLog{}
	sd := newShutdown()
	sd.add("close TUN", 20*time.Millisecond, blockForever(l, "tun"))
	sd.add("remove routes", 20*time.Millisecond, blockForever(l, "routes"))
	sd.restoreDNSFirst("restore system DNS", 20*time.Millisecond, recordStep(l, "dns", 0))
	sd.add("close connect client", 20*time.Millisecond, blockForever(l, "client"))

	var logged string
	start := time.Now()
	logged = captureStderr(t, func() { sd.run(nil) })
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("shutdown took %v; three blocked 20ms steps must not hold it", elapsed)
	}
	if got := l.all(); !slices.Equal(got, []string{"start dns", "done dns", "start client", "start routes", "start tun"}) {
		t.Fatalf("steps = %v; every step must still get its turn", got)
	}
	for _, name := range []string{"close connect client", "remove routes", "close TUN"} {
		if !strings.Contains(logged, name) {
			t.Fatalf("stderr = %q; want the timed-out step %q named", logged, name)
		}
	}
}

func TestShutdown_TotalBudgetBoundsIt(t *testing.T) {
	l := &leakLog{}
	sd := newShutdown()
	sd.total = 60 * time.Millisecond
	for _, name := range []string{"a", "b", "c", "d"} {
		sd.add(name, time.Hour, blockForever(l, name))
	}

	start := time.Now()
	captureStderr(t, func() { sd.run(nil) })

	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("shutdown took %v; want it cut off near the 60ms total", elapsed)
	}
}

func TestShutdown_DNSTimeoutReportsNotRestored(t *testing.T) {
	sd := newShutdown()
	sd.restoreDNSFirst("restore system DNS", 20*time.Millisecond, blockForever(&leakLog{}, "dns"))

	var ok bool
	logged := captureStderr(t, func() { ok = sd.run(nil) })

	if ok {
		t.Fatal("run = true; DNS restore never finished")
	}
	if !strings.Contains(logged, "sudo urnet-client recover") {
		t.Fatalf("stderr = %q; a DNS restore that did not finish must name the recover command", logged)
	}
}

func TestShutdown_DNSErrorReportsNotRestored(t *testing.T) {
	sd := newShutdown()
	sd.restoreDNSFirst("restore system DNS", time.Second, func() error { return errors.New("scutil: Permission denied") })

	var ok bool
	captureStderr(t, func() { ok = sd.run(nil) })

	if ok {
		t.Fatal("run = true; DNS restore failed")
	}
}

func TestShutdown_WithoutDNSStepCountsAsRestored(t *testing.T) {
	if !newShutdown().run(nil) {
		t.Fatal("run = false; a run that never changed DNS has nothing to restore")
	}
}

func TestShutdown_SecondSignalWaitsForDNSThenSkipsTheRest(t *testing.T) {
	l := &leakLog{}
	sd := newShutdown()
	sd.add("remove routes", time.Second, recordStep(l, "routes", 0))
	sd.restoreDNSFirst("restore system DNS", time.Second, recordStep(l, "dns", 50*time.Millisecond))
	sd.add("close connect client", time.Second, blockForever(l, "client"))
	abort := make(chan struct{})
	close(abort)

	var ok bool
	logged := captureStderr(t, func() { ok = sd.run(abort) })

	if !ok || !slices.Equal(l.all(), []string{"start dns", "done dns"}) {
		t.Fatalf("run = %v, steps = %v; want DNS restored in full and nothing after it", ok, l.all())
	}
	if !strings.Contains(logged, "close connect client") || !strings.Contains(logged, "remove routes") {
		t.Fatalf("stderr = %q; want the skipped steps named", logged)
	}
}

func TestShutdown_SecondSignalCutsARunningStepShort(t *testing.T) {
	l := &leakLog{}
	sd := newShutdown()
	sd.add("remove routes", time.Hour, blockForever(l, "routes"))
	abort := make(chan struct{})
	time.AfterFunc(30*time.Millisecond, func() { close(abort) })

	start := time.Now()
	captureStderr(t, func() { sd.run(abort) })

	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("shutdown took %v after a second signal", elapsed)
	}
}

func TestLeakGuard_RegistersDNSRestoreFirst(t *testing.T) {
	f := newLeakFixture()
	g, err := apply(t, f, leakCfg("none"))
	if err != nil {
		t.Fatal(err)
	}
	sd := newShutdown()
	sd.add("close connect client", time.Second, func() error { f.log.add("client: close"); return nil })

	g.register(sd)
	sd.run(nil)

	got := f.log.all()
	if !slices.Equal(got[len(got)-3:], []string{"sysdns: restore", "stub: close", "client: close"}) {
		t.Fatalf("steps = %v; want DNS restored before anything else, then the stub closed", got)
	}
}
