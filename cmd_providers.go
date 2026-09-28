package main

import (
	"context"
	"fmt"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/urnetwork/connect"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

func cmdFindProviders(ctx context.Context, opts docopt.Opts) error {
	apiURL := config.StringOr(opts, "--api_url", config.DefaultAPIURL)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	if clientID := parseClientID(jwt); clientID != "" {
		fmt.Printf("client_id: %s\n", clientID)
	}

	count := config.IntOr(opts, "--count", 8)
	rankMode := config.StringOr(opts, "--rank_mode", "quality")

	qCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	loc := config.ParseLocationConfig(opts)
	_, specs := buildProviderSpecs(qCtx, apiURL, jwt, loc)

	api := newByAPI(qCtx, apiURL, jwt)

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

// idsToStrings converts a slice of connect.Id to a slice of their string representations.
func idsToStrings(ids []connect.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
