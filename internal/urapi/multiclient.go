package urapi

import (
	"context"
	"fmt"
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
	mc packetSender
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

func (m *MultiClient) SendPacket(packet []byte) {
	start := time.Now()
	ok := m.mc.SendPacket(connect.TransferPath{}, protocol.ProvideMode_Network, packet, sendTimeout)
	if waited := time.Since(start); waited > slowSendWarning {
		logx.Warn("sending a packet waited %s for a provider window\n", waited.Round(time.Millisecond))
	}
	if !ok {
		logx.Debug("dropped outbound packet len=%d: no provider took it within %s\n", len(packet), sendTimeout)
	}
}

// Close stops the client's transports and provider windows.
func (m *MultiClient) Close() error {
	m.mc.Close()
	return nil
}
