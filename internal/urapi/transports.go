package urapi

import (
	"context"
	"fmt"

	"github.com/urnetwork/connect"
)

// OpenTransports opens n platform transports for clientID. The returned func closes the
// transports in reverse order, then the client.
func OpenTransports(ctx context.Context, apiURL, connectURL, jwt, clientID string, n int, appVersion string) (func(), error) {
	api := newByAPI(ctx, apiURL, jwt)
	strat := connect.NewClientStrategyWithDefaults(ctx)
	id, err := connect.ParseId(clientID)
	if err != nil {
		return nil, err
	}
	oob := connect.NewApiOutOfBandControlWithApi(api)
	client := connect.NewClientWithDefaults(ctx, id, oob)
	clientAuth := &connect.ClientAuth{
		ByJwt:      jwt,
		InstanceId: connect.NewId(),
		AppVersion: fmt.Sprintf("urnet-client %s", appVersion),
	}
	var transports []*connect.PlatformTransport
	for range n {
		transports = append(transports, connect.NewPlatformTransportWithDefaults(ctx, strat, client.RouteManager(), fmt.Sprintf("%s/", connectURL), clientAuth))
	}
	return func() {
		for i := len(transports) - 1; i >= 0; i-- {
			transports[i].Close()
		}
		client.Close()
	}, nil
}
