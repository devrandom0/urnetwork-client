package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
)

// cmdQuickConnect performs: optional login+verify → ensure client JWT (with refresh) → start VPN.
func cmdQuickConnect(opts docopt.Opts) {
	if bg, _ := opts.Bool("--background"); bg {
		pid, err := spawnBackground(os.Args)
		if err != nil {
			fatal(fmt.Errorf("background start failed: %w", err))
		}
		fmt.Printf("started in background pid=%d\n", pid)
		return
	}

	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)

	if logPath := strings.TrimSpace(getStringOr(opts, "--log_file", "")); logPath != "" {
		if err := setupLogFile(logPath); err != nil {
			fatal(fmt.Errorf("log setup failed: %w", err))
		}
	}
	lvl := strings.TrimSpace(getStringOr(opts, "--log_level", ""))
	dbg, _ := opts.Bool("--debug")
	setLogLevel(lvl, dbg)

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
			fatal(errors.New("--user_auth and --password must be provided together"))
		}
		loginRes, loginErr := loginWithPassword(apiUrl, userAuth, password)
		if loginErr != nil {
			logError("login error: %v\n", loginErr)
			os.Exit(1)
		}
		if loginRes.VerificationRequired {
			if codeOpt == "" {
				logError("verification required (re-run with --code=<code> or run 'verify')\n")
				os.Exit(1)
			}
			// Verification required + code provided → proceed to verify step below.
			// Network JWT is not available yet; don't save.
		} else {
			if loginRes.ByJwt == "" {
				logError("login succeeded but no by_jwt returned\n")
				os.Exit(1)
			}
			if err := saveJWT(loginRes.ByJwt); err != nil {
				logError("save jwt failed: %v\n", err)
				os.Exit(1)
			}
			logInfo("saved JWT for network %s -> %s\n", loginRes.NetworkName, jwtPath())
		}

		// Verify if a code was provided (either because it was always present or VerificationRequired)
		if codeOpt != "" {
			byJwt2, verifyErr := verifyCode(apiUrl, userAuth, codeOpt)
			if verifyErr != nil {
				logError("verify error: %v\n", verifyErr)
				os.Exit(1)
			}
			if err := saveJWT(byJwt2); err != nil {
				logError("save jwt failed: %v\n", err)
				os.Exit(1)
			}
			logInfo("verified and saved JWT -> %s\n", jwtPath())
		}
	}

	// 2) Ensure we have a working client-scoped JWT
	{
		jwt, err := loadJWT(jwtOpt)
		if err != nil {
			fatal(errors.New("no JWT available; provide --user_auth/--password to login or --jwt to use an existing token"))
		}

		if id := parseClientID(jwt); id != "" && !forceJWT {
			if validateClientJWT(apiUrl, jwt) {
				logInfo("using existing client JWT (client_id=%s)\n", id)
			} else {
				// Invalid client JWT → retry with credentials or exit.
				retryEvery := renewInterval
				if retryEvery <= 0 {
					retryEvery = time.Minute
				}
				for {
					if userAuth == "" || password == "" {
						fatal(errors.New("existing client JWT appears invalid; provide --user_auth and --password or a BY token via --jwt to refresh"))
					}
					loginRes, loginErr := loginWithPassword(apiUrl, userAuth, password)
					if loginErr != nil {
						logWarn("jwt refresh: login failed: %v\n", loginErr)
					} else if !loginRes.VerificationRequired && loginRes.ByJwt != "" {
						clientJwt, mintErr := mintClientJWT(apiUrl, loginRes.ByJwt)
						if mintErr != nil {
							logWarn("jwt refresh: mint failed: %v\n", mintErr)
						} else if saveErr := saveJWT(clientJwt); saveErr != nil {
							logWarn("jwt refresh: save failed: %v\n", saveErr)
						} else if validateClientJWT(apiUrl, clientJwt) {
							logInfo("obtained new client JWT; proceeding\n")
							break
						}
					}
					logWarn("jwt still not usable; retrying in %s\n", retryEvery.String())
					time.Sleep(retryEvery)
				}
			}
		} else {
			// Network-scoped token or --force_jwt: mint a fresh client JWT.
			clientJwt, mintErr := mintClientJWT(apiUrl, jwt)
			if mintErr != nil {
				fatal(mintErr)
			}
			if err := saveJWT(clientJwt); err != nil {
				fatal(err)
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
					// Try to mint a fresh client JWT from the current saved token.
					currentJwt, err := loadJWT("")
					if err != nil {
						logWarn("jwt renew: no jwt available: %v\n", err)
						continue
					}
					clientJwt, mintErr := mintClientJWT(apiUrl, currentJwt)
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
					// Fallback: re-login with credentials and mint.
					if userAuth != "" && password != "" {
						loginRes, loginErr := loginWithPassword(apiUrl, userAuth, password)
						if loginErr != nil {
							logWarn("jwt renew: login failed: %v\n", loginErr)
							continue
						}
						if loginRes.VerificationRequired || loginRes.ByJwt == "" {
							logWarn("jwt renew: login requires verification or returned no JWT\n")
							continue
						}
						clientJwt2, mintErr2 := mintClientJWT(apiUrl, loginRes.ByJwt)
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

	// 4) Start VPN in the foreground
	cmdVpn(opts)
	close(stopRenew)
}
