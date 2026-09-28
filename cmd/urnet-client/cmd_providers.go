package main

import (
	"context"
	"fmt"
	"time"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/auth"
	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

func cmdFindProviders(ctx context.Context, opts docopt.Opts) error {
	apiURL := config.StringOr(opts, "--api_url", config.DefaultAPIURL)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := auth.Load(jwtOpt)
	if err != nil {
		return err
	}

	if clientID := auth.ParseClientID(jwt); clientID != "" {
		fmt.Printf("client_id: %s\n", clientID)
	}

	count := config.IntOr(opts, "--count", 8)
	rankMode := config.StringOr(opts, "--rank_mode", "quality")

	qCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	specs := urapi.SelectProviders(qCtx, apiURL, jwt, config.ParseLocationConfig(opts))
	if specs.BestAvailableOnly() {
		fmt.Println("using best-available providers")
	}

	providers, err := urapi.FindProviders(qCtx, apiURL, jwt, specs, count, rankMode)
	if err != nil {
		return err
	}
	for _, p := range providers {
		fmt.Printf("provider client_id=%s tier=%d est_bps=%d intermediaries=%v\n",
			p.ClientID, p.Tier, p.EstimatedBytesPerSecond, p.IntermediaryIDs)
	}
	return nil
}
