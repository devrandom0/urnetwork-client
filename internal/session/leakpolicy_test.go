package session

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/dnsstub"
	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/tunnel"
)

// leakLog records every step the fakes see, in order, across routes, stub and system DNS.
type leakLog struct {
	mu    sync.Mutex
	steps []string
}

func (l *leakLog) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.steps = append(l.steps, s)
}

func (l *leakLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.steps...)
}

func (l *leakLog) has(s string) bool { return slices.Contains(l.all(), s) }

type fakeLeakRoutes struct {
	log                 *leakLog
	rejectErr, splitErr error
}

func (f *fakeLeakRoutes) AddIPv6Reject() error {
	f.log.add("routes: ipv6 reject")
	return f.rejectErr
}

func (f *fakeLeakRoutes) AddIPv6SplitDefault() error {
	f.log.add("routes: ipv6 via tun")
	return f.splitErr
}

func (f *fakeLeakRoutes) AddDNSServerRoutes(ips []string, bypass bool) {
	if bypass {
		f.log.add("routes: dns bypass " + strings.Join(ips, ","))
	}
}

func (f *fakeLeakRoutes) RemoveDNSBypass() { f.log.add("routes: remove dns bypass") }

type fakeStub struct {
	log  *leakLog
	opts dnsstub.Options
}

func (f *fakeStub) Addr() netip.AddrPort { return f.opts.Listen }
func (f *fakeStub) BindUpstream()        { f.log.add("stub: bind upstream") }
func (f *fakeStub) Close() error {
	f.log.add("stub: close")
	return nil
}

type fakeSysDNS struct {
	log      *leakLog
	applyErr error
}

func (f *fakeSysDNS) Apply(stub netip.Addr) error {
	f.log.add("sysdns: apply " + stub.String())
	return f.applyErr
}

func (f *fakeSysDNS) Restore() error {
	f.log.add("sysdns: restore")
	return nil
}

type leakFixture struct {
	log     *leakLog
	routes  *fakeLeakRoutes
	sysDNS  *fakeSysDNS
	stub    *fakeStub
	stubErr error
	v6      bool
}

func newLeakFixture() *leakFixture {
	l := &leakLog{}
	return &leakFixture{log: l, routes: &fakeLeakRoutes{log: l}, sysDNS: &fakeSysDNS{log: l}, v6: true}
}

func (f *leakFixture) deps() leakDeps {
	return leakDeps{
		routes: f.routes,
		sysDNS: f.sysDNS,
		startStub: func(o dnsstub.Options) (stubServer, error) {
			f.log.add("stub: start")
			if f.stubErr != nil {
				return nil, f.stubErr
			}
			f.stub = &fakeStub{log: f.log, opts: o}
			return f.stub, nil
		},
		control:       func(string, string, syscall.RawConn) error { return nil },
		ipv6Available: func() bool { return f.v6 },
	}
}

func leakCfg(mode string) config.VPNConfig {
	return config.VPNConfig{IPCIDR: "10.255.0.2/24", DefaultRoute: true, DNSBootstrap: mode}
}

func apply(t *testing.T, f *leakFixture, cfg config.VPNConfig) (*leakGuard, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return applyLeakPolicy(ctx, cfg, nil, &tunnel.Counters{}, f.deps())
}

func TestLeakPolicy_DefaultBlocksIPv6AndBindsStub(t *testing.T) {
	f := newLeakFixture()

	g, err := apply(t, f, leakCfg("none"))

	if err != nil {
		t.Fatalf("applyLeakPolicy: %v", err)
	}
	want := []string{"routes: ipv6 reject", "stub: start", "sysdns: apply 10.255.0.2"}
	if got := f.log.all(); !slices.Equal(got, want) {
		t.Fatalf("steps = %v; want %v", got, want)
	}
	o := f.stub.opts
	if o.Listen.String() != "10.255.0.2:53" || !o.BindUpstream || !o.FilterAAAA || o.Control == nil {
		t.Fatalf("stub options = %+v; want the TUN address, bound upstreams and AAAA filtering", o)
	}
	if got := addrStrings(o.Upstreams); !slices.Equal(got, []string{"1.1.1.1", "9.9.9.9"}) {
		t.Fatalf("upstreams = %v; want the defaults when --dns is unset", got)
	}
	if g.StubAddr() != "10.255.0.2:53" {
		t.Fatalf("StubAddr = %q", g.StubAddr())
	}
}

func TestLeakPolicy_EnableIPv6RoutesIPv6AndKeepsAAAA(t *testing.T) {
	f := newLeakFixture()
	cfg := leakCfg("none")
	cfg.EnableIPv6 = true

	if _, err := apply(t, f, cfg); err != nil {
		t.Fatal(err)
	}

	if !f.log.has("routes: ipv6 via tun") || f.log.has("routes: ipv6 reject") || f.stub.opts.FilterAAAA {
		t.Fatalf("steps = %v filterAAAA=%v; want IPv6 routed into the TUN and AAAA passed", f.log.all(), f.stub.opts.FilterAAAA)
	}
}

func TestLeakPolicy_NoRejectRoutesWhenKernelHasNoIPv6(t *testing.T) {
	f := newLeakFixture()
	f.v6 = false

	if _, err := apply(t, f, leakCfg("none")); err != nil {
		t.Fatal(err)
	}
	if f.log.has("routes: ipv6 reject") {
		t.Fatal("reject routes installed although the kernel has IPv6 off")
	}
}

func TestLeakPolicy_IPv6RouteFailureStopsBeforeDNS(t *testing.T) {
	f := newLeakFixture()
	f.routes.rejectErr = errors.New("RTNETLINK answers: Permission denied")

	if _, err := apply(t, f, leakCfg("none")); err == nil {
		t.Fatal("applyLeakPolicy = nil; want the IPv6 route error")
	}
	if f.log.has("stub: start") || f.log.has("sysdns: apply 10.255.0.2") {
		t.Fatalf("steps = %v; nothing after the failed IPv6 step may run", f.log.all())
	}
}

func TestLeakPolicy_StubFailureLeavesSystemDNSAlone(t *testing.T) {
	f := newLeakFixture()
	f.stubErr = errors.New("listen udp 10.255.0.2:53: bind: address already in use")

	if _, err := apply(t, f, leakCfg("none")); err == nil {
		t.Fatal("applyLeakPolicy = nil; want the bind error")
	}
	if f.log.has("sysdns: apply 10.255.0.2") {
		t.Fatal("system DNS pointed at a stub that is not running")
	}
}

func TestLeakPolicy_SystemDNSFailureClosesStub(t *testing.T) {
	f := newLeakFixture()
	f.sysDNS.applyErr = errors.New("scutil: Permission denied")

	if _, err := apply(t, f, leakCfg("none")); err == nil {
		t.Fatal("applyLeakPolicy = nil; want the system DNS error")
	}
	if got := f.log.all(); got[len(got)-1] != "stub: close" {
		t.Fatalf("steps = %v; want the stub closed after system DNS failed", got)
	}
}

func TestLeakPolicy_BypassRoutesUpstreamsAndWarns(t *testing.T) {
	logx.SetLogLevel("info", false)
	f := newLeakFixture()
	cfg := leakCfg("bypass")
	cfg.DNSList = "9.9.9.9, 149.112.112.112:5353"

	logged := captureStderr(t, func() {
		if _, err := apply(t, f, cfg); err != nil {
			t.Error(err)
		}
	})

	if !f.log.has("routes: dns bypass 9.9.9.9,149.112.112.112") || f.stub.opts.BindUpstream {
		t.Fatalf("steps = %v bind=%v; want host routes around the tunnel and unbound upstreams", f.log.all(), f.stub.opts.BindUpstream)
	}
	if got := f.stub.opts.Upstreams[1].String(); got != "149.112.112.112:5353" {
		t.Fatalf("upstream = %s; want the port from --dns kept", got)
	}
	if !strings.Contains(logged, "leave outside the tunnel") {
		t.Fatalf("stderr = %q; bypass mode must warn that DNS leaks", logged)
	}
}

func TestLeakPolicy_CacheBindsStubBeforeDroppingBypass(t *testing.T) {
	f := newLeakFixture()
	counters := &tunnel.Counters{PktsIn: 1, PktsOut: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := applyLeakPolicy(ctx, leakCfg("cache"), nil, counters, f.deps()); err != nil {
		t.Fatal(err)
	}
	if f.stub.opts.BindUpstream {
		t.Fatal("cache mode must start with unbound upstreams")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !f.log.has("routes: remove dns bypass") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	got := f.log.all()
	bind, drop := slices.Index(got, "stub: bind upstream"), slices.Index(got, "routes: remove dns bypass")
	if bind < 0 || drop < 0 || bind > drop {
		t.Fatalf("steps = %v; want the stub bound before the bypass routes go", got)
	}
}

func TestLeakGuard_CloseRestoresDNSBeforeStoppingStub(t *testing.T) {
	f := newLeakFixture()
	g, err := apply(t, f, leakCfg("none"))
	if err != nil {
		t.Fatal(err)
	}

	g.Close()

	got := f.log.all()
	if !slices.Equal(got[len(got)-2:], []string{"sysdns: restore", "stub: close"}) {
		t.Fatalf("steps = %v; want DNS restored, then the stub closed", got)
	}
}

func TestPlanDNS(t *testing.T) {
	cases := []struct {
		name, cidr, dns string
		ipv6            bool
		wantErr         string
	}{
		{"ipv6 upstream while blocked", "10.255.0.2/24", "2606:4700::1111", false, "IPv6"},
		{"ipv6 upstream allowed", "10.255.0.2/24", "2606:4700::1111", true, ""},
		{"not an address", "10.255.0.2/24", "dns.example", false, "not an IP"},
		{"ipv6 tun address", "fd00::2/120", "", false, "IPv4 TUN address"},
		{"bad cidr", "10.255.0.2", "", false, "IPv4 TUN address"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := planDNS(config.VPNConfig{IPCIDR: tc.cidr, DNSList: tc.dns, EnableIPv6: tc.ipv6})
			if tc.wantErr == "" && err != nil {
				t.Fatalf("planDNS = %v; want nil", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("planDNS = %v; want an error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestEndpointPins_Add(t *testing.T) {
	pins := endpointPins{}
	ips := []netip.Addr{netip.MustParseAddr("203.0.113.7")}
	pins.add("api.example.test", ips)
	pins.add("203.0.113.9", ips)
	pins.add("connect.example.test", nil)
	pins.add("", ips)
	if len(pins) != 1 || pins["api.example.test"][0] != ips[0] {
		t.Fatalf("pins = %v; want only the named host with addresses", pins)
	}
}
