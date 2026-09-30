package urapi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/connect/protocol"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
)

type GeneratorConfig struct {
	APIURL     string
	ConnectURL string
	JWT        string
	AppVersion string
	Location   config.LocationConfig
}

// Generator picks providers for a MultiClient.
type Generator struct {
	g *connect.ApiMultiClientGenerator
}

func NewGenerator(ctx context.Context, c GeneratorConfig) *Generator {
	strat, specs := buildProviderSpecs(ctx, c.APIURL, c.JWT, c.Location)
	appVer := fmt.Sprintf("urnet-client %s", c.AppVersion)
	return &Generator{g: connect.NewApiMultiClientGeneratorWithDefaults(
		ctx, specs, strat, nil, c.APIURL, c.JWT, fmt.Sprintf("%s/", c.ConnectURL), "", "", appVer, nil,
	)}
}

// packetSender is the part of connect's multi-client that MultiClient uses.
type packetSender interface {
	SendPacket(source connect.TransferPath, provideMode protocol.ProvideMode, packet []byte, timeout time.Duration) bool
	Close()
}

// MultiClient carries IP packets to and from the selected providers.
type MultiClient struct {
	mc       packetSender
	slowWarn warnLimiter
}

func NewMultiClient(ctx context.Context, gen *Generator, receive func(packet []byte)) *MultiClient {
	mc := connect.NewRemoteUserNatMultiClientWithDefaults(ctx, gen.g, func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		logx.Debug("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		receive(packet)
	}, protocol.ProvideMode_Network)
	return &MultiClient{mc: mc}
}

// sendTimeout bounds the wait for a provider window. connect waits forever on a negative
// timeout, which let one packet sent before any provider was up block the TUN reader.
const sendTimeout = 2 * time.Second

var slowSendWarning = time.Second

// slowSendWarnEvery caps the slow-send WARN: in an outage every send is slow for as long as
// it lasts.
const slowSendWarnEvery = 30 * time.Second

func (m *MultiClient) SendPacket(packet []byte) {
	start := time.Now()
	ok := m.mc.SendPacket(connect.TransferPath{}, protocol.ProvideMode_Network, packet, sendTimeout)
	waited := time.Since(start)
	if waited > slowSendWarning {
		m.warnSlowSend(waited)
	}
	if !ok {
		if waited >= sendTimeout {
			logx.Debug("dropped outbound packet len=%d: no provider window within %s\n", len(packet), sendTimeout)
		} else {
			logx.Debug("dropped outbound packet len=%d: the connect client refused it (unparseable, ICMP, or blocked by its egress policy)\n", len(packet))
		}
	}
}

func (m *MultiClient) warnSlowSend(waited time.Duration) {
	ok, suppressed := m.slowWarn.allow(time.Now())
	if !ok {
		return
	}
	more := ""
	if suppressed > 0 {
		more = fmt.Sprintf(" (%d more slow sends since the last warning)", suppressed)
	}
	logx.Warn("sending a packet waited %s for a provider window%s\n", waited.Round(time.Millisecond), more)
}

// warnLimiter lets one warning through per slowSendWarnEvery and counts the rest.
type warnLimiter struct {
	mu         sync.Mutex
	last       time.Time
	suppressed int
}

func (l *warnLimiter) allow(now time.Time) (ok bool, suppressed int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.last.IsZero() && now.Sub(l.last) < slowSendWarnEvery {
		l.suppressed++
		return false, 0
	}
	suppressed, l.suppressed, l.last = l.suppressed, 0, now
	return true, suppressed
}

// Close stops the client's transports and provider windows.
func (m *MultiClient) Close() error {
	m.mc.Close()
	return nil
}
