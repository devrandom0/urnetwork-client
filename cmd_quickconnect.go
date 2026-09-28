package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/auth"
	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/session"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

// jwtLoadArgForStep2 decides what to pass to loadJWT when ensuring the client JWT after
// the optional login step. When login just ran and the JWT only came from URNETWORK_JWT,
// it forces a fresh read from disk so a stale env token cannot shadow the JWT login
// just saved. An explicit --jwt always wins.
func jwtLoadArgForStep2(jwtOpt string, jwtFromEnv bool, userAuth, password string) string {
	if jwtFromEnv && (userAuth != "" || password != "") {
		return ""
	}
	return jwtOpt
}

// cmdQuickConnect performs: optional login+verify → ensure client JWT (with refresh) → start VPN.
func cmdQuickConnect(ctx context.Context, opts docopt.Opts, jwtFromEnv bool) error {
	vpnCfg, err := config.ResolveVPNConfig(opts)
	if err != nil {
		return err
	}
	apiURL := vpnCfg.APIURL

	userAuth := strings.TrimSpace(config.StringOr(opts, "--user_auth", ""))
	password := strings.TrimSpace(config.StringOr(opts, "--password", ""))
	if userAuth == "" {
		userAuth = strings.TrimSpace(os.Getenv("URNETWORK_USERNAME"))
	}
	if password == "" {
		password = strings.TrimSpace(os.Getenv("URNETWORK_PASSWORD"))
	}
	codeOpt := strings.TrimSpace(config.StringOr(opts, "--code", ""))
	jwtOpt, _ := opts.String("--jwt")
	forceJWT, _ := opts.Bool("--force_jwt")
	renewStr := strings.TrimSpace(config.StringOr(opts, "--jwt_renew_interval", ""))
	var renewInterval time.Duration
	if renewStr != "" {
		if d, err := time.ParseDuration(renewStr); err == nil {
			renewInterval = d
		} else {
			logx.Warn("invalid --jwt_renew_interval=%q; ignoring\n", renewStr)
		}
	}

	// 1) Login if credentials provided
	if userAuth != "" || password != "" {
		if userAuth == "" || password == "" {
			return errors.New("--user_auth and --password must be provided together")
		}
		loginRes, loginErr := urapi.LoginWithPassword(ctx, apiURL, userAuth, password)
		if loginErr != nil {
			return fmt.Errorf("login error: %w", loginErr)
		}
		if loginRes.VerificationRequired {
			if codeOpt == "" {
				return errors.New("verification required (re-run with --code=<code> or run 'verify')")
			}
			// Verification required + code provided → proceed to verify step below.
		} else {
			if loginRes.ByJwt == "" {
				return errors.New("login succeeded but no by_jwt returned")
			}
			if err := auth.Save(loginRes.ByJwt); err != nil {
				return fmt.Errorf("save jwt failed: %w", err)
			}
			logx.Info("saved JWT for network %s -> %s\n", loginRes.NetworkName, auth.Path())
		}

		if codeOpt != "" {
			byJwt2, verifyErr := urapi.VerifyCode(ctx, apiURL, userAuth, codeOpt)
			if verifyErr != nil {
				return fmt.Errorf("verify error: %w", verifyErr)
			}
			if err := auth.Save(byJwt2); err != nil {
				return fmt.Errorf("save jwt failed: %w", err)
			}
			logx.Info("verified and saved JWT -> %s\n", auth.Path())
		}
	}

	// 2) Ensure we have a working client-scoped JWT
	jwt, err := auth.Load(jwtLoadArgForStep2(jwtOpt, jwtFromEnv, userAuth, password))
	if err != nil {
		return errors.New("no JWT available; provide --user_auth/--password to login or --jwt to use an existing token")
	}
	clientJWT, err := auth.EnsureClientJWT(ctx, apiURL, jwt, forceJWT, userAuth, password)
	if err != nil {
		return err
	}

	// 3) Optional background JWT renewal goroutine
	stopRenew := make(chan struct{})
	if renewInterval > 0 {
		go auth.RenewLoop(ctx, apiURL, userAuth, password, renewInterval, stopRenew)
	}

	// 4) Start VPN with the client JWT from step 2
	vpnCfg.JWT = clientJWT
	runErr := session.Run(ctx, vpnCfg, Version)
	close(stopRenew)
	return runErr
}
