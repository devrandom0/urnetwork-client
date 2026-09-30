package dnsstub

import (
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
)

func answerAAAA(ip string) func(dns.ResponseWriter, *dns.Msg, bool) {
	return func(w dns.ResponseWriter, req *dns.Msg, _ bool) {
		m := new(dns.Msg).SetReply(req)
		q := req.Question[0]
		if q.Qtype == dns.TypeAAAA {
			m.Answer = append(m.Answer, &dns.AAAA{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60}, AAAA: net.ParseIP(ip)})
		}
		_ = w.WriteMsg(m)
	}
}

func TestStub_FiltersAAAAWhenIPv6Off(t *testing.T) {
	up := startUpstream(t, answerAAAA("2001:db8::1"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, FilterAAAA: true})

	r := query(t, s, "udp", "example.test", dns.TypeAAAA)

	if r.Rcode != dns.RcodeSuccess || len(r.Answer) != 0 {
		t.Fatalf("rcode=%s answers=%v; want an empty NOERROR so apps fall back to IPv4 at once", dns.RcodeToString[r.Rcode], r.Answer)
	}
	if got := up.networks(); len(got) != 0 {
		t.Fatalf("upstream saw %v; a filtered AAAA query must not be forwarded", got)
	}
}

func TestStub_PassesAAAAWhenIPv6On(t *testing.T) {
	up := startUpstream(t, answerAAAA("2001:db8::1"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}})

	if r := query(t, s, "udp", "example.test", dns.TypeAAAA); len(r.Answer) != 1 {
		t.Fatalf("answers = %v; want the upstream's AAAA record with --enable_ipv6", r.Answer)
	}
}

func TestStub_FilterAAAAKeepsOtherTypes(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.4"))
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, FilterAAAA: true})

	if r := query(t, s, "udp", "example.test", dns.TypeA); len(r.Answer) != 1 {
		t.Fatalf("answers = %v; A queries must pass while AAAA is filtered", r.Answer)
	}
}

func TestStub_AnswersPinnedNameLocally(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.99"))
	s := startStub(t, Options{
		Upstreams: []netip.AddrPort{up.addr},
		Pinned:    map[string][]netip.Addr{"API.Example.Test": {netip.MustParseAddr("192.0.2.50")}},
	})

	r := query(t, s, "udp", "api.example.TEST", dns.TypeA)

	if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "192.0.2.50" {
		t.Fatalf("answers = %v; want the pinned startup address", r.Answer)
	}
	if got := up.networks(); len(got) != 0 {
		t.Fatalf("upstream saw %v; pinned names must not depend on the tunnel", got)
	}
}

func TestStub_PinnedNameHasNoAAAA(t *testing.T) {
	up := startUpstream(t, answerAAAA("2001:db8::1"))
	s := startStub(t, Options{
		Upstreams: []netip.AddrPort{up.addr},
		Pinned:    map[string][]netip.Addr{"api.example.test": {netip.MustParseAddr("192.0.2.50")}},
	})

	if r := query(t, s, "udp", "api.example.test", dns.TypeAAAA); len(r.Answer) != 0 {
		t.Fatalf("answers = %v; the bypass routes are IPv4 only", r.Answer)
	}
}

func TestSourceAllowed(t *testing.T) {
	local := func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("10.255.0.2"), Mask: net.CIDRMask(24, 32)}}, nil
	}
	failing := func() ([]net.Addr, error) { return nil, errors.New("no interfaces") }
	cases := []struct {
		name  string
		ip    string
		addrs func() ([]net.Addr, error)
		want  bool
	}{
		{"ipv4 loopback", "127.0.0.1", failing, true},
		{"ipv6 loopback", "::1", failing, true},
		{"own TUN address", "10.255.0.2", local, true},
		{"mapped own address", "::ffff:10.255.0.2", local, true},
		{"other host", "10.255.0.7", local, false},
		{"interface lookup fails", "10.255.0.2", failing, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceAllowed(netip.MustParseAddr(tc.ip), tc.addrs); got != tc.want {
				t.Fatalf("sourceAllowed(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}
