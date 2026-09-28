package urapi

import (
	"context"
	"fmt"

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

// MultiClient carries IP packets to and from the selected providers.
type MultiClient struct {
	mc *connect.RemoteUserNatMultiClient
}

func NewMultiClient(ctx context.Context, gen *Generator, receive func(packet []byte)) *MultiClient {
	mc := connect.NewRemoteUserNatMultiClientWithDefaults(ctx, gen.g, func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		logx.Debug("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		receive(packet)
	}, protocol.ProvideMode_Network)
	return &MultiClient{mc: mc}
}

func (m *MultiClient) SendPacket(packet []byte) {
	m.mc.SendPacket(connect.TransferPath{}, protocol.ProvideMode_Network, packet, -1)
}
