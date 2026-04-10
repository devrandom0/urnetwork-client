package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
)

// cmdQuickConnect performs: optional login+verify → ensure client JWT (with refresh) → start VPN.
func cmdQuickConnect(ctx context.Context, opts docopt.Opts) error {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)

	userAuth := strings.TrimSpace(getStringOr(opts, "--user_auth", ""))
	password := strings.TrimSpace(getStringOr(opts, "--password", ""))
	if userAuth == "" {
		userAuth = strings.TrimSpace(os.Getenv("URNETWORK_USERNAME"))
	}
	if password == "" {
		password = strings.TrimSpace(os.Getenv("URNETWORK_PASSWORD"))
	}
	codeOpt := strings.TrimSpace(getStringOr(opts, "--code", ""))
	jwtOpt, _ := opts.String("--jwt")
	forceJWT, _ := opts.Bool("--force_jwt")
	renewStr := strings.TrimSpace(getStringOr(opts, "--jwt_renew_interval", ""))
	var renewInterval time.Duration
	if renewStr != "" {
		if d, err := time.ParseDuration(renewStr); err == nil {
			renewInterval = d
		} else {
			logWarn("invalid --jwt_renew_interval=%q; ignoring\n", renewStr)
		}
	}

	// 1) Login if credentials provided
	if userAuth != "" || password != "" {
		if userAuth == "" || password == "" {
			return errors.New("--user_auth and --password must be provided together")
		}
		loginRes, loginErr := loginWithPassword(ctx, apiUrl, userAuth, password)
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
			if err := saveJWT(loginRes.ByJwt); err != nil {
				return fmt.Errorf("save jwt failed: %w", err)
			}
			logInfo("saved JWT for network %s -> %s\n", loginRes.NetworkName, jwtPath())
		}

		if codeOpt != "" {
			byJwt2, verifyErr := verifyCode(ctx, apiUrl, userAuth, codeOpt)
			if verifyErr != nil {
				return fmt.Errorf("verify error: %w", verifyErr)
			}
			if err := saveJWT(byJwt2); err != nil {
				return fmt.Errorf("save jwt failed: %w", err)
			}
			logInfo("verified and saved JWT -> %s\n", jwtPath())
		}
	}

	// 2) Ensure we have a working client-scoped JWT
	{
		jwt, err := loadJWT(jwtOpt)
		if err != nil {
			return errors.New("no JWT available; provide --user_auth/--password to login or --jwt to use an existing token")
		}

		if id := parseClientID(jwt); id != "" && !forceJWT {
			if validateClientJWT(ctx, apiUrl, jwt) {
				logInfo("using existing client JWT (client_id=%s)\n", id)
			} else {
				retryEvery := renewInterval
				if retryEvery <= 0 {
					retryEvery = time.Minute
				}
				for {
					if userAuth == "" || password == "" {
						return errors.New("existing client JWT appears invalid; provide --user_auth and --password or a BY token via --jwt to refresh")
					}
					loginRes, loginErr := loginWithPassword(ctx, apiUrl, userAuth, password)
					if loginErr != nil {
						logWarn("jwt refresh: login failed: %v\n", loginErr)
					} else if !loginRes.VerificationRequired && loginRes.ByJwt != "" {
						clientJwt, mintErr := mintClientJWT(ctx, apiUrl, loginRes.ByJwt)
						if mintErr != nil {
							logWarn("jwt refresh: mint failed: %v\n", mintErr)
						} else if saveErr := saveJWT(clientJwt); saveErr != nil {
							logWarn("jwt refresh: save failed: %v\n", saveErr)
						} else if validateClientJWT(ctx, apiUrl, clientJwt) {
							logInfo("obtained new client JWT; proceeding\n")
							break
						}
					}
					logWarn("jwt still not usable; retrying in %s\n", retryEvery.String())
					select {
					case <-time.After(retryEvery):
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
		} else {
			clientJwt, mintErr := mintClientJWT(ctx, apiUrl, jwt)
			if mintErr != nil {
				return mintErr
			}
			if err := saveJWT(clientJwt); err != nil {
				return err
			}
			if id := parseClientID(clientJwt); id != "" {
				logInfo("saved client JWT (client_id=%s) -> %s\n", id, jwtPath())
			} else {
				logInfo("saved client JWT -> %s\n", jwtPath())
			}
		}
	}

	// 3) Optional background JWT renewal goroutine
	stopRenew := make(chan struct{})
	if renewInterval > 0 {
		go func() {
			ticker := time.NewTicker(renewInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					currentJwt, err := loadJWT("")
					if err != nil {
						logWarn("jwt renew: no jwt available: %v\n", err)
						continue
					}
					clientJwt, mintErr := mintClientJWT(ctx, apiUrl, currentJwt)
					if mintErr == nil {
						if saveErr := saveJWT(clientJwt); saveErr != nil {
							logWarn("jwt renew: save failed: %v\n", saveErr)
						} else if id := parseClientID(clientJwt); id != "" {
							logInfo("jwt renewed (client_id=%s)\n", id)
						} else {
							logInfo("jwt renewed\n")
						}
						continue
					}
					if userAuth != "" && password != "" {
						loginRes, loginErr := loginWithPassword(ctx, apiUrl, userAuth, password)
						if loginErr != nil {
							logWarn("jwt renew: login failed: %v\n", loginErr)
							continue
						}
						if loginRes.VerificationRequired || loginRes.ByJwt == "" {
							logWarn("jwt renew: login requires verification or returned no JWT\n")
							continue
						}
						clientJwt2, mintErr2 := mintClientJWT(ctx, apiUrl, loginRes.ByJwt)
						if mintErr2 != nil {
							logWarn("jwt renew: mint failed: %v\n", mintErr2)
							continue
						}
						if saveErr := saveJWT(clientJwt2); saveErr != nil {
							logWarn("jwt renew: save failed: %v\n", saveErr)
						} else if id := parseClientID(clientJwt2); id != "" {
							logInfo("jwt renewed (client_id=%s)\n", id)
						} else {
							logInfo("jwt renewed\n")
						}
					}
				case <-stopRenew:
					return
				}
			}
		}()
	}

	// 4) Load the final JWT and start VPN
	finalJWT, err := loadJWT("")
	if err != nil {
		close(stopRenew)
		return fmt.Errorf("no jwt available after setup: %w", err)
	}
	vpnCfg := parseVPNConfig(opts, finalJWT)
	runErr := cmdVpn(ctx, vpnCfg)
	close(stopRenew)
	return runErr
}
