package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/docopt/docopt-go"
)

func cmdSaveJWT(opts docopt.Opts) error {
	jwt, _ := opts.String("--jwt")
	if strings.TrimSpace(jwt) == "" {
		return fmt.Errorf("--jwt is required")
	}
	if err := saveJWT(jwt); err != nil {
		return fmt.Errorf("save failed: %w", err)
	}
	fmt.Printf("saved to %s\n", jwtPath())
	return nil
}

func cmdVerify(ctx context.Context, opts docopt.Opts) error {
	apiURL := getStringOr(opts, "--api_url", DefaultAPIURL)
	userAuth, _ := opts.String("--user_auth")
	code, _ := opts.String("--code")

	byJwt, err := verifyCode(ctx, apiURL, userAuth, code)
	if err != nil {
		return fmt.Errorf("verify error: %w", err)
	}
	if err := saveJWT(byJwt); err != nil {
		return fmt.Errorf("save jwt failed: %w", err)
	}
	fmt.Printf("saved JWT -> %s\n", jwtPath())
	return nil
}

func cmdMintClient(ctx context.Context, opts docopt.Opts) error {
	apiURL := getStringOr(opts, "--api_url", DefaultAPIURL)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	clientJwt, err := mintClientJWT(ctx, apiURL, jwt)
	if err != nil {
		return err
	}
	if err := saveJWT(clientJwt); err != nil {
		return err
	}
	if id := parseClientID(clientJwt); id != "" {
		fmt.Printf("saved client JWT (client_id=%s) -> %s\n", id, jwtPath())
	} else {
		fmt.Printf("saved client JWT -> %s\n", jwtPath())
	}
	return nil
}
