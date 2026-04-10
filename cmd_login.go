package main

import (
	"fmt"
	"os"

	"github.com/docopt/docopt-go"
)

func cmdLogin(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	userAuth := mustString(opts, "--user_auth")
	password := mustString(opts, "--password")

	res, err := loginWithPassword(apiUrl, userAuth, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login error: %v\n", err)
		os.Exit(1)
	}
	if res.VerificationRequired {
		fmt.Printf("verification required for %s\n", userAuth)
		return
	}
	if res.ByJwt == "" {
		fmt.Fprintln(os.Stderr, "login succeeded but no by_jwt returned")
		os.Exit(1)
	}
	if err := saveJWT(res.ByJwt); err != nil {
		fmt.Fprintf(os.Stderr, "save jwt failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("saved JWT for network %s -> %s\n", res.NetworkName, jwtPath())
}
