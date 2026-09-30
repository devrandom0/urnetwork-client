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

func v6Answers(w dns.ResponseWriter, req *dns.Msg, _ bool) {
	m := new(dns.Msg).SetReply(req)
	name := req.Question[0].Name
	hdr := func(t uint16) dns.RR_Header {
		return dns.RR_Header{Name: name, Rrtype: t, Class: dns.ClassINET, Ttl: 60}
	}
	m.Answer = append(m.Answer,
		aRecord(name, "192.0.2.6"),
		&dns.AAAA{Hdr: hdr(dns.TypeAAAA), AAAA: net.ParseIP("2001:db8::6")},
		&dns.HTTPS{SVCB: dns.SVCB{Hdr: hdr(dns.TypeHTTPS), Priority: 1, Target: ".", Value: []dns.SVCBKeyValue{
			&dns.SVCBAlpn{Alpn: []string{"h2"}},
			&dns.SVCBIPv4Hint{Hint: []net.IP{net.ParseIP("192.0.2.6").To4()}},
			&dns.SVCBIPv6Hint{Hint: []net.IP{net.ParseIP("2001:db8::6")}},
		}}},
	)
	m.Extra = append(m.Extra, &dns.AAAA{Hdr: hdr(dns.TypeAAAA), AAAA: net.ParseIP("2001:db8::7")})
	_ = w.WriteMsg(m)
}

func hasIPv6(m *dns.Msg) string {
	for _, rr := range append(append(append([]dns.RR{}, m.Answer...), m.Ns...), m.Extra...) {
		switch v := rr.(type) {
		case *dns.AAAA:
			return v.String()
		case *dns.HTTPS:
			for _, kv := range v.Value {
				if kv.Key() == dns.SVCB_IPV6HINT {
					return v.String()
				}
			}
		}
	}
	return ""
}

func TestStub_StripsIPv6FromForwardedAnswersWhenIPv6Off(t *testing.T) {
	up := startUpstream(t, v6Answers)
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}, FilterAAAA: true})

	for _, qtype := range []uint16{dns.TypeANY, dns.TypeHTTPS} {
		r := query(t, s, "tcp", "example.test", qtype)
		if rr := hasIPv6(r); rr != "" {
			t.Fatalf("%s reply still carries %q; apps would try IPv6 into the reject routes", dns.TypeToString[qtype], rr)
		}
		if len(r.Answer) != 2 {
			t.Fatalf("%s answers = %v; want the A and HTTPS records kept", dns.TypeToString[qtype], r.Answer)
		}
	}
}

func TestStub_KeepsIPv6InForwardedAnswersWhenIPv6On(t *testing.T) {
	up := startUpstream(t, v6Answers)
	s := startStub(t, Options{Upstreams: []netip.AddrPort{up.addr}})

	if r := query(t, s, "tcp", "example.test", dns.TypeANY); hasIPv6(r) == "" {
		t.Fatalf("answers = %v; want AAAA passed with --enable_ipv6", r.Answer)
	}
}

func TestAnswer_ReportsWhereTheAnswerCameFrom(t *testing.T) {
	up := startUpstream(t, answerA("192.0.2.99"))
	s := startStub(t, Options{
		Upstreams: []netip.AddrPort{up.addr},
		Pinned:    map[string][]netip.Addr{"api.example.test": {netip.MustParseAddr("192.0.2.50")}},
	})
	for name, want := range map[string]string{"api.example.test": "local", "other.example.test": "forward"} {
		if _, path := s.answer(new(dns.Msg).SetQuestion(dns.Fqdn(name), dns.TypeA), false); path != want {
			t.Fatalf("%s answered by %q, want %q", name, path, want)
		}
	}
}

func TestQueryLogLine(t *testing.T) {
	req := new(dns.Msg).SetQuestion("example.test.", dns.TypeAAAA)
	got := queryLogLine(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5353}, req, "local")
	if got != "dnsstub: 127.0.0.1:5353 example.test. AAAA local" {
		t.Fatalf("log line = %q", got)
	}
}
