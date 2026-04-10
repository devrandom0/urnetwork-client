package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/docopt/docopt-go"
	"github.com/urnetwork/connect"
)

func cmdOpen(ctx context.Context, opts docopt.Opts) error {
	apiURL := getStringOr(opts, "--api_url", DefaultAPIURL)
	connectURL := getStringOr(opts, "--connect_url", DefaultConnectURL)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	api := newByAPI(ctx, apiURL, jwt)
	strat := connect.NewClientStrategyWithDefaults(ctx)

	clientIDStr := parseClientID(jwt)
	if clientIDStr == "" {
		return errors.New("JWT missing client_id (run 'urnet-client mint-client' to mint a client-scoped JWT)")
	}
	clientID, err := connect.ParseId(clientIDStr)
	if err != nil {
		return err
	}

	oob := connect.NewApiOutOfBandControlWithApi(api)
	client := connect.NewClientWithDefaults(ctx, clientID, oob)
	defer client.Close()

	auth := &connect.ClientAuth{
		ByJwt:      jwt,
		InstanceId: connect.NewId(),
		AppVersion: fmt.Sprintf("urnet-client %s", Version),
	}

	n := getIntOr(opts, "--transports", 4)
	for i := 0; i < n; i++ {
		pt := connect.NewPlatformTransportWithDefaults(ctx, strat, client.RouteManager(), fmt.Sprintf("%s/", connectURL), auth)
		defer pt.Close()
	}

	fmt.Println("transports opened; press Ctrl-C to exit")
	<-ctx.Done()
	return nil
}
