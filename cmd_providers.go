package main

import (
	"context"
	"fmt"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/urnetwork/connect"
)

func cmdFindProviders(ctx context.Context, opts docopt.Opts) error {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	if clientID := parseClientID(jwt); clientID != "" {
		fmt.Printf("client_id: %s\n", clientID)
	}

	count := getIntOr(opts, "--count", 8)
	rankMode := getStringOr(opts, "--rank_mode", "quality")

	qCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	loc := parseLocationConfig(opts)
	_, specs := buildProviderSpecs(qCtx, apiUrl, jwt, loc)

	api := newByAPI(qCtx, apiUrl, jwt)

	if len(specs) == 1 && specs[0].BestAvailable {
		fmt.Println("using best-available providers")
	}

	res, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs, Count: count, RankMode: rankMode})
	if err != nil {
		return err
	}
	for _, p := range res.Providers {
		fmt.Printf("provider client_id=%s tier=%d est_bps=%d intermediaries=%v\n",
			p.ClientId.String(), p.Tier, p.EstimatedBytesPerSecond, idsToStrings(p.IntermediaryIds))
	}
	return nil
}
