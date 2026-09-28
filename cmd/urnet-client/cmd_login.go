package main

import (
	"context"
	"fmt"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/auth"
	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

func cmdLogin(ctx context.Context, opts docopt.Opts) error {
	apiURL := config.StringOr(opts, "--api_url", config.DefaultAPIURL)
	userAuth, _ := opts.String("--user_auth")
	password, _ := opts.String("--password")

	res, err := urapi.LoginWithPassword(ctx, apiURL, userAuth, password)
	if err != nil {
		return fmt.Errorf("login error: %w", err)
	}
	if res.VerificationRequired {
		fmt.Printf("verification required for %s\n", userAuth)
		return nil
	}
	if res.ByJwt == "" {
		return fmt.Errorf("login succeeded but no by_jwt returned")
	}
	if err := auth.Save(res.ByJwt); err != nil {
		return fmt.Errorf("save jwt failed: %w", err)
	}
	fmt.Printf("saved JWT for network %s -> %s\n", res.NetworkName, auth.Path())
	return nil
}
