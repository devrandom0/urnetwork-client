package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/auth"
	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

func cmdOpen(ctx context.Context, opts docopt.Opts) error {
	apiURL := config.StringOr(opts, "--api_url", config.DefaultAPIURL)
	connectURL := config.StringOr(opts, "--connect_url", config.DefaultConnectURL)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := auth.Load(jwtOpt)
	if err != nil {
		return err
	}

	clientIDStr := auth.ParseClientID(jwt)
	if clientIDStr == "" {
		return errors.New("JWT missing client_id (run 'urnet-client mint-client' to mint a client-scoped JWT)")
	}
	closeAll, err := urapi.OpenTransports(ctx, apiURL, connectURL, jwt, clientIDStr, config.IntOr(opts, "--transports", 4), Version)
	if err != nil {
		return err
	}
	defer closeAll()

	fmt.Println("transports opened; press Ctrl-C to exit")
	<-ctx.Done()
	return nil
}
