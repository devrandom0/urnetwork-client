package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/logx"
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
	vpnCfg, err := resolveVPNConfig(opts)
	if err != nil {
		return err
	}
	apiURL := vpnCfg.APIURL

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
			logx.Warn("invalid --jwt_renew_interval=%q; ignoring\n", renewStr)
		}
	}

	// 1) Login if credentials provided
	if userAuth != "" || password != "" {
		if userAuth == "" || password == "" {
			return errors.New("--user_auth and --password must be provided together")
		}
		loginRes, loginErr := loginWithPassword(ctx, apiURL, userAuth, password)
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
			logx.Info("saved JWT for network %s -> %s\n", loginRes.NetworkName, jwtPath())
		}

		if codeOpt != "" {
			byJwt2, verifyErr := verifyCode(ctx, apiURL, userAuth, codeOpt)
			if verifyErr != nil {
				return fmt.Errorf("verify error: %w", verifyErr)
			}
			if err := saveJWT(byJwt2); err != nil {
				return fmt.Errorf("save jwt failed: %w", err)
			}
			logx.Info("verified and saved JWT -> %s\n", jwtPath())
		}
	}

	// 2) Ensure we have a working client-scoped JWT
	jwt, err := loadJWT(jwtLoadArgForStep2(jwtOpt, jwtFromEnv, userAuth, password))
	if err != nil {
		return errors.New("no JWT available; provide --user_auth/--password to login or --jwt to use an existing token")
	}
	clientJWT, err := ensureClientJWT(ctx, apiURL, jwt, forceJWT, userAuth, password)
	if err != nil {
		return err
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
						logx.Warn("jwt renew: no jwt available: %v\n", err)
						continue
					}
					clientJwt, mintErr := mintClientJWT(ctx, apiURL, currentJwt)
					if mintErr == nil {
						if saveErr := saveJWT(clientJwt); saveErr != nil {
							logx.Warn("jwt renew: save failed: %v\n", saveErr)
						} else if id := parseClientID(clientJwt); id != "" {
							logx.Info("jwt renewed (client_id=%s)\n", id)
						} else {
							logx.Info("jwt renewed\n")
						}
						continue
					}
					if userAuth != "" && password != "" {
						loginRes, loginErr := loginWithPassword(ctx, apiURL, userAuth, password)
						if loginErr != nil {
							logx.Warn("jwt renew: login failed: %v\n", loginErr)
							continue
						}
						if loginRes.VerificationRequired || loginRes.ByJwt == "" {
							logx.Warn("jwt renew: login requires verification or returned no JWT\n")
							continue
						}
						clientJwt2, mintErr2 := mintClientJWT(ctx, apiURL, loginRes.ByJwt)
						if mintErr2 != nil {
							logx.Warn("jwt renew: mint failed: %v\n", mintErr2)
							continue
						}
						if saveErr := saveJWT(clientJwt2); saveErr != nil {
							logx.Warn("jwt renew: save failed: %v\n", saveErr)
						} else if id := parseClientID(clientJwt2); id != "" {
							logx.Info("jwt renewed (client_id=%s)\n", id)
						} else {
							logx.Info("jwt renewed\n")
						}
					}
				case <-stopRenew:
					return
				}
			}
		}()
	}

	// 4) Start VPN with the client JWT from step 2
	vpnCfg.JWT = clientJWT
	runErr := cmdVpn(ctx, vpnCfg)
	close(stopRenew)
	return runErr
}

const (
	loginRetryMin = 10 * time.Second
	loginRetryMax = 5 * time.Minute
)

// loginRetryBackoff is independent of --jwt_renew_interval, which is usually hours and
// used to stall startup for that long after a single failed login.
func loginRetryBackoff(attempt int) time.Duration {
	d := loginRetryMin
	for i := 0; i < attempt && d < loginRetryMax; i++ {
		d *= 2
	}
	return min(d, loginRetryMax)
}

// ensureClientJWT returns a usable client-scoped JWT, starting from jwt: a valid client JWT
// is used as is, an invalid one is refreshed by logging in again, and a BY network JWT is
// exchanged for a client JWT. Refreshed and minted tokens are also saved to disk.
func ensureClientJWT(ctx context.Context, apiURL, jwt string, forceJWT bool, userAuth, password string) (string, error) {
	id := parseClientID(jwt)
	if id == "" || forceJWT {
		clientJwt, err := mintClientJWT(ctx, apiURL, jwt)
		if err != nil {
			return "", err
		}
		if err := saveJWT(clientJwt); err != nil {
			return "", err
		}
		if id := parseClientID(clientJwt); id != "" {
			logx.Info("saved client JWT (client_id=%s) -> %s\n", id, jwtPath())
		} else {
			logx.Info("saved client JWT -> %s\n", jwtPath())
		}
		return clientJwt, nil
	}
	if validateClientJWT(ctx, apiURL, jwt) {
		logx.Info("using existing client JWT (client_id=%s)\n", id)
		return jwt, nil
	}
	for attempt := 0; ; attempt++ {
		if userAuth == "" || password == "" {
			return "", errors.New("existing client JWT appears invalid; provide --user_auth and --password or a BY token via --jwt to refresh")
		}
		loginRes, loginErr := loginWithPassword(ctx, apiURL, userAuth, password)
		if loginErr != nil {
			logx.Warn("jwt refresh: login failed: %v\n", loginErr)
		} else if !loginRes.VerificationRequired && loginRes.ByJwt != "" {
			clientJwt, mintErr := mintClientJWT(ctx, apiURL, loginRes.ByJwt)
			if mintErr != nil {
				logx.Warn("jwt refresh: mint failed: %v\n", mintErr)
			} else if saveErr := saveJWT(clientJwt); saveErr != nil {
				logx.Warn("jwt refresh: save failed: %v\n", saveErr)
			} else if validateClientJWT(ctx, apiURL, clientJwt) {
				logx.Info("obtained new client JWT; proceeding\n")
				return clientJwt, nil
			}
		}
		wait := loginRetryBackoff(attempt)
		logx.Warn("jwt still not usable; retrying in %s\n", wait)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}
