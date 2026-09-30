package session

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"syscall"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/dnsstub"
	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
	"github.com/devrandom0/urnetwork-client/internal/tunnel"
)

var defaultStubUpstreams = []string{"1.1.1.1", "9.9.9.9"}

// leakRoutes is the part of the route managers the leak policy drives.
type leakRoutes interface {
	AddIPv6Reject() error
	AddIPv6SplitDefault() error
	AddDNSServerRoutes(ips []string, bypass bool)
	RemoveDNSBypass()
}

type stubServer interface {
	Addr() netip.AddrPort
	BindUpstream()
	Close() error
}

type leakDeps struct {
	routes    leakRoutes
	sysDNS    netcfg.SystemDNS
	startStub func(dnsstub.Options) (stubServer, error)
	control   func(network, address string, c syscall.RawConn) error
}

// dnsPlan is what --default_route needs for DNS: where the stub listens, where it forwards,
// and which upstream path (--dns_bootstrap) it starts with.
type dnsPlan struct {
	listen    netip.AddrPort
	upstreams []netip.AddrPort
	mode      string
}

func planDNS(cfg config.VPNConfig) (dnsPlan, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cfg.IPCIDR))
	if err != nil || !prefix.Addr().Is4() {
		return dnsPlan{}, fmt.Errorf("--ip_cidr %q: the DNS stub needs an IPv4 TUN address", cfg.IPCIDR)
	}
	servers := config.SplitCSV(cfg.DNSList)
	if len(servers) == 0 {
		servers = defaultStubUpstreams
	}
	var ups []netip.AddrPort
	for _, s := range servers {
		ap, err := parseUpstream(s)
		if err != nil {
			return dnsPlan{}, fmt.Errorf("--dns %q: %w", s, err)
		}
		if ap.Addr().Is6() && !cfg.EnableIPv6 {
			return dnsPlan{}, fmt.Errorf("--dns %q is IPv6, but IPv6 is blocked without --enable_ipv6", s)
		}
		ups = append(ups, ap)
	}
	mode := cfg.DNSBootstrap
	if mode == "" {
		mode = config.DefaultDNSBootstrap
	}
	return dnsPlan{listen: netip.AddrPortFrom(prefix.Addr(), 53), upstreams: ups, mode: mode}, nil
}

func parseUpstream(s string) (netip.AddrPort, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.AddrPort{}, errors.New("not an IP address or ip:port")
	}
	return netip.AddrPortFrom(a.Unmap(), 53), nil
}

// preflight runs before the first route or kill-switch change: a config the DNS stub cannot
// use must fail while the host is untouched, and stale DNS from a crashed run is recovered
// in every mode. Only a run that points system DNS at its own stub (tunMode with
// --default_route) needs that recovery to succeed; the others only warn.
func preflight(cfg config.VPNConfig, tunMode bool, recoverStaleDNS func() error) error {
	ownsDNS := tunMode && cfg.DefaultRoute
	if ownsDNS {
		if _, err := planDNS(cfg); err != nil {
			return err
		}
	}
	if err := recoverStaleDNS(); err != nil {
		if ownsDNS {
			return err
		}
		logx.Warn("system DNS: %v\n", err)
	}
	return nil
}

// leakGuard undoes the DNS part of the leak policy; the route manager's Cleanup owns routes.
type leakGuard struct {
	stub   stubServer
	sysDNS netcfg.SystemDNS
}

func (g *leakGuard) StubAddr() string { return g.stub.Addr().String() }

// register makes system DNS the first thing restored, so no lookup meets a dead resolver.
func (g *leakGuard) register(sd *shutdown) {
	sd.restoreDNSFirst("restore system DNS", dnsRestoreTimeout, g.sysDNS.Restore)
	sd.add("stop DNS stub", stopStubTimeout, g.stub.Close)
}

// applyLeakPolicy runs after the IPv4 split routes are in place. Every step fails closed and
// undoes the DNS steps before it; routes it added stay for the route manager's Cleanup.
func applyLeakPolicy(ctx context.Context, cfg config.VPNConfig, pinned map[string][]netip.Addr, counters *tunnel.Counters, d leakDeps) (*leakGuard, error) {
	plan, err := planDNS(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.EnableIPv6 {
		err = d.routes.AddIPv6SplitDefault()
	} else {
		err = d.routes.AddIPv6Reject()
	}
	if err != nil {
		return nil, err
	}
	bypass := plan.mode != "none"
	if bypass {
		d.routes.AddDNSServerRoutes(addrStrings(plan.upstreams), true)
	}
	if plan.mode == "bypass" {
		logx.Warn("--dns_bootstrap=bypass: all traffic to %s, DNS and anything else such as DNS over HTTPS, goes outside the tunnel for the whole session\n", strings.Join(addrStrings(plan.upstreams), ", "))
	}
	stub, err := d.startStub(dnsstub.Options{
		Listen:       plan.listen,
		Upstreams:    plan.upstreams,
		Control:      d.control,
		BindUpstream: !bypass,
		FilterAAAA:   !cfg.EnableIPv6,
		Pinned:       pinned,
	})
	if err != nil {
		return nil, fmt.Errorf("DNS stub: %w", err)
	}
	if err := d.sysDNS.Apply(plan.listen.Addr()); err != nil {
		_ = stub.Close()
		return nil, fmt.Errorf("point system DNS at the stub: %w", err)
	}
	if plan.mode == "cache" {
		go netcfg.RemoveDNSBypassWhenWarm(ctx, bindThenUnroute{d.routes, stub}, &counters.PktsIn, &counters.PktsOut, 3*time.Second, 200*time.Millisecond)
	}
	return &leakGuard{stub: stub, sysDNS: d.sysDNS}, nil
}

// bindThenUnroute pins the stub to the tunnel before the bypass routes go, so no query can
// take the physical path in between.
type bindThenUnroute struct {
	routes leakRoutes
	stub   stubServer
}

func (b bindThenUnroute) RemoveDNSBypass() {
	b.stub.BindUpstream()
	b.routes.RemoveDNSBypass()
}

// endpointPins maps control-plane hosts to the addresses routed around the tunnel at startup.
type endpointPins map[string][]netip.Addr

// add takes AddBypassEndpoint's results as they come; IP-literal hosts need no DNS answer.
func (p endpointPins) add(host string, ips []netip.Addr) {
	if host == "" || len(ips) == 0 {
		return
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return
	}
	p[host] = ips
}

func addrStrings(aps []netip.AddrPort) []string {
	out := make([]string, len(aps))
	for i, ap := range aps {
		out[i] = ap.Addr().String()
	}
	return out
}
