package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/urnetwork/connect"
)

func cmdOpen(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
	api.SetByJwt(jwt)

	clientIDStr := parseClientID(jwt)
	if clientIDStr == "" {
		fatal(errors.New("JWT missing client_id (run 'urnet-client mint-client' to mint a client-scoped JWT)"))
	}
	clientID, err := connect.ParseId(clientIDStr)
	if err != nil {
		fatal(err)
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
		pt := connect.NewPlatformTransportWithDefaults(ctx, strat, client.RouteManager(), fmt.Sprintf("%s/", connectUrl), auth)
		defer pt.Close()
	}

	fmt.Println("transports opened; press Ctrl-C to exit")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	_ = time.Duration(0) // keep import
}
