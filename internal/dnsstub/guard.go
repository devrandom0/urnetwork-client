package dnsstub

import (
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// guardedListener screens TCP clients before the DNS server spends a goroutine on them, and
// caps how many it holds open, so hosts that can reach the TUN address cannot exhaust it.
type guardedListener struct {
	net.Listener
	allowed   func(netip.Addr) bool
	slots     chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func newGuardedListener(ln net.Listener, allowed func(netip.Addr) bool, maxConns int) *guardedListener {
	return &guardedListener{Listener: ln, allowed: allowed, slots: make(chan struct{}, maxConns), done: make(chan struct{})}
}

func (l *guardedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			<-l.slots
			return nil, err
		}
		if !l.allowed(addrIP(c.RemoteAddr())) {
			_ = c.Close()
			continue
		}
		return &slotConn{Conn: c, release: sync.OnceFunc(func() { <-l.slots })}, nil
	}
}

func (l *guardedListener) Close() error {
	l.closeOnce.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type slotConn struct {
	net.Conn
	release func()
}

func (c *slotConn) Close() error {
	err := c.Conn.Close()
	c.release()
	return err
}

// localAddrCache limits interface listing to once per ttl; every query checks its source,
// and listing interfaces per packet would let a flood cost a syscall each.
type localAddrCache struct {
	list    func() ([]net.Addr, error)
	now     func() time.Time
	ttl     time.Duration
	mu      sync.Mutex
	fetched time.Time
	addrs   []net.Addr
	err     error
}

func (c *localAddrCache) get() ([]net.Addr, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now := c.now(); c.fetched.IsZero() || now.Sub(c.fetched) >= c.ttl {
		c.addrs, c.err = c.list()
		c.fetched = now
	}
	return c.addrs, c.err
}

// stripIPv6 removes IPv6 addresses from a forwarded answer while IPv6 is blocked: AAAA records
// in any section and ipv6hint in HTTPS and SVCB records.
func stripIPv6(m *dns.Msg) {
	m.Answer = withoutIPv6(m.Answer)
	m.Ns = withoutIPv6(m.Ns)
	m.Extra = withoutIPv6(m.Extra)
}

func withoutIPv6(rrs []dns.RR) []dns.RR {
	out := rrs[:0]
	for _, rr := range rrs {
		switch v := rr.(type) {
		case *dns.AAAA:
			continue
		case *dns.SVCB:
			v.Value = withoutIPv6Hint(v.Value)
		case *dns.HTTPS:
			v.Value = withoutIPv6Hint(v.Value)
		}
		out = append(out, rr)
	}
	return out
}

func withoutIPv6Hint(kvs []dns.SVCBKeyValue) []dns.SVCBKeyValue {
	out := kvs[:0]
	for _, kv := range kvs {
		if kv.Key() != dns.SVCB_IPV6HINT {
			out = append(out, kv)
		}
	}
	return out
}
