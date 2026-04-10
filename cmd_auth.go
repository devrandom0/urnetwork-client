package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/docopt/docopt-go"
)

func cmdSaveJWT(opts docopt.Opts) {
	jwt, _ := opts.String("--jwt")
	if strings.TrimSpace(jwt) == "" {
		fmt.Fprintln(os.Stderr, "--jwt is required")
		os.Exit(2)
	}
	if err := saveJWT(jwt); err != nil {
		fmt.Fprintf(os.Stderr, "save failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("saved to %s\n", jwtPath())
}

func cmdVerify(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	userAuth := mustString(opts, "--user_auth")
	code := mustString(opts, "--code")

	byJwt, err := verifyCode(apiUrl, userAuth, code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify error: %v\n", err)
		os.Exit(1)
	}
	if err := saveJWT(byJwt); err != nil {
		fmt.Fprintf(os.Stderr, "save jwt failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("saved JWT -> %s\n", jwtPath())
}

func cmdMintClient(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	clientJwt, err := mintClientJWT(apiUrl, jwt)
	if err != nil {
		fatal(err)
	}
	if err := saveJWT(clientJwt); err != nil {
		fatal(err)
	}
	if id := parseClientID(clientJwt); id != "" {
		fmt.Printf("saved client JWT (client_id=%s) -> %s\n", id, jwtPath())
	} else {
		fmt.Printf("saved client JWT -> %s\n", jwtPath())
	}
}
