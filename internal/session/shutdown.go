package session

import (
	"strings"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
)

// Per-step deadlines for the VPN's cleanup. A step that misses its deadline keeps running in
// the background while the next one starts; the process exits soon after anyway.
const (
	dnsRestoreTimeout   = 3 * time.Second
	stopListenerTimeout = time.Second
	stopStubTimeout     = 2 * time.Second
	clientCloseTimeout  = 5 * time.Second
	routeCleanupTimeout = 5 * time.Second
	closeTUNTimeout     = 2 * time.Second
	shutdownBudget      = 15 * time.Second
)

type cleanupStep struct {
	name    string
	timeout time.Duration
	run     func() error
}

// shutdown undoes a session's changes with system DNS first: while DNS points at a stub that
// is stopping, every lookup on the host fails, whereas routes and the tunnel only affect
// traffic that is already ending.
type shutdown struct {
	total time.Duration
	dns   *cleanupStep
	steps []cleanupStep
}

func newShutdown() *shutdown { return &shutdown{total: shutdownBudget} }

func (s *shutdown) restoreDNSFirst(name string, timeout time.Duration, fn func() error) {
	s.dns = &cleanupStep{name: name, timeout: timeout, run: fn}
}

// add registers a step; steps run last-added first, like defers.
func (s *shutdown) add(name string, timeout time.Duration, fn func() error) {
	s.steps = append(s.steps, cleanupStep{name: name, timeout: timeout, run: fn})
}

// run reports whether system DNS is back to what it was. A closed abort (a second signal)
// never cuts the DNS step short, but skips every step after it.
func (s *shutdown) run(abort <-chan struct{}) (dnsRestored bool) {
	deadline := time.Now().Add(s.total)
	dnsRestored = true
	if s.dns != nil {
		dnsRestored = s.runDNS(deadline)
	}
	for i := len(s.steps) - 1; i >= 0; i-- {
		select {
		case <-abort:
			s.logSkipped(i)
			return dnsRestored
		default:
		}
		st := s.steps[i]
		if err, done := runStep(st, deadline, abort); !done {
			logx.Warn("cleanup: %s did not finish within %s; continuing\n", st.name, st.timeout)
		} else if err != nil {
			logx.Error("cleanup: %s: %v\n", st.name, err)
		}
	}
	return dnsRestored
}

func (s *shutdown) runDNS(deadline time.Time) bool {
	err, done := runStep(*s.dns, deadline, nil)
	switch {
	case !done:
		logx.Error("cleanup: %s did not finish within %s; %s\n", s.dns.name, s.dns.timeout, netcfg.RecoverHint)
		return false
	case err != nil:
		msg := err.Error()
		if !strings.Contains(msg, netcfg.RecoverHint) {
			msg += "; " + netcfg.RecoverHint
		}
		logx.Error("cleanup: %s: %s\n", s.dns.name, msg)
		return false
	}
	return true
}

func (s *shutdown) logSkipped(last int) {
	var names []string
	for i := last; i >= 0; i-- {
		names = append(names, s.steps[i].name)
	}
	logx.Warn("cleanup: second signal, skipping: %s\n", strings.Join(names, ", "))
}

// runStep reports done=false when the step's own timeout, the total deadline or abort came first.
func runStep(st cleanupStep, deadline time.Time, abort <-chan struct{}) (err error, done bool) {
	wait := st.timeout
	if left := time.Until(deadline); left < wait {
		wait = left
	}
	if wait <= 0 {
		return nil, false
	}
	result := make(chan error, 1)
	go func() { result <- st.run() }()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case err := <-result:
		return err, true
	case <-timer.C:
		return nil, false
	case <-abort:
		return nil, false
	}
}

type releaser interface{ Release() }

// stayArmed keeps the watchdog's pipe open until this process exits but never releases it,
// for a startup failure whose DNS rollback may itself have failed: the watchdog then checks.
type stayArmed struct{ releaser }

func (stayArmed) Release() {}

// runShutdown runs sd with a second signal able to skip the steps after DNS.
func runShutdown(sd *shutdown, wd releaser) {
	abort, stop := watchSecondSignal()
	defer stop()
	finishShutdown(sd, abort, wd)
}

// finishShutdown keeps the watchdog armed when DNS is not confirmed restored: once this
// process exits, it retries the restore for us.
func finishShutdown(sd *shutdown, abort <-chan struct{}, wd releaser) {
	if sd.run(abort) && wd != nil {
		wd.Release()
	}
}
