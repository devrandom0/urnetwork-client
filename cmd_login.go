package main

import (
	"context"
	"fmt"

	"github.com/docopt/docopt-go"
)

func cmdLogin(ctx context.Context, opts docopt.Opts) error {
	apiURL := getStringOr(opts, "--api_url", DefaultAPIURL)
	userAuth, _ := opts.String("--user_auth")
	password, _ := opts.String("--password")

	res, err := loginWithPassword(ctx, apiURL, userAuth, password)
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
	if err := saveJWT(res.ByJwt); err != nil {
		return fmt.Errorf("save jwt failed: %w", err)
	}
	fmt.Printf("saved JWT for network %s -> %s\n", res.NetworkName, jwtPath())
	return nil
}
