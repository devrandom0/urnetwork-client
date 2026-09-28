package auth

import (
	"context"
	"errors"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

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

// EnsureClientJWT returns a usable client-scoped JWT, starting from jwt: a valid client JWT
// is used as is, an invalid one is refreshed by logging in again, and a BY network JWT is
// exchanged for a client JWT. Refreshed and minted tokens are also saved to disk.
func EnsureClientJWT(ctx context.Context, apiURL, jwt string, forceJWT bool, userAuth, password string) (string, error) {
	id := ParseClientID(jwt)
	if id == "" || forceJWT {
		clientJwt, err := urapi.MintClientJWT(ctx, apiURL, jwt)
		if err != nil {
			return "", err
		}
		if err := Save(clientJwt); err != nil {
			return "", err
		}
		if id := ParseClientID(clientJwt); id != "" {
			logx.Info("saved client JWT (client_id=%s) -> %s\n", id, Path())
		} else {
			logx.Info("saved client JWT -> %s\n", Path())
		}
		return clientJwt, nil
	}
	if urapi.ValidateClientJWT(ctx, apiURL, jwt) {
		logx.Info("using existing client JWT (client_id=%s)\n", id)
		return jwt, nil
	}
	for attempt := 0; ; attempt++ {
		if userAuth == "" || password == "" {
			return "", errors.New("existing client JWT appears invalid; provide --user_auth and --password or a BY token via --jwt to refresh")
		}
		loginRes, loginErr := urapi.LoginWithPassword(ctx, apiURL, userAuth, password)
		if loginErr != nil {
			logx.Warn("jwt refresh: login failed: %v\n", loginErr)
		} else if !loginRes.VerificationRequired && loginRes.ByJwt != "" {
			clientJwt, mintErr := urapi.MintClientJWT(ctx, apiURL, loginRes.ByJwt)
			if mintErr != nil {
				logx.Warn("jwt refresh: mint failed: %v\n", mintErr)
			} else if saveErr := Save(clientJwt); saveErr != nil {
				logx.Warn("jwt refresh: save failed: %v\n", saveErr)
			} else if urapi.ValidateClientJWT(ctx, apiURL, clientJwt) {
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

// RenewLoop re-mints the client JWT every interval until stop is closed. When minting from
// the stored JWT fails and credentials are set, it logs in again first.
func RenewLoop(ctx context.Context, apiURL, userAuth, password string, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			renewOnce(ctx, apiURL, userAuth, password)
		case <-stop:
			return
		}
	}
}

func renewOnce(ctx context.Context, apiURL, userAuth, password string) {
	currentJwt, err := Load("")
	if err != nil {
		logx.Warn("jwt renew: no jwt available: %v\n", err)
		return
	}
	clientJwt, mintErr := urapi.MintClientJWT(ctx, apiURL, currentJwt)
	if mintErr == nil {
		saveRenewed(clientJwt)
		return
	}
	if userAuth == "" || password == "" {
		return
	}
	loginRes, loginErr := urapi.LoginWithPassword(ctx, apiURL, userAuth, password)
	if loginErr != nil {
		logx.Warn("jwt renew: login failed: %v\n", loginErr)
		return
	}
	if loginRes.VerificationRequired || loginRes.ByJwt == "" {
		logx.Warn("jwt renew: login requires verification or returned no JWT\n")
		return
	}
	clientJwt2, mintErr2 := urapi.MintClientJWT(ctx, apiURL, loginRes.ByJwt)
	if mintErr2 != nil {
		logx.Warn("jwt renew: mint failed: %v\n", mintErr2)
		return
	}
	saveRenewed(clientJwt2)
}

func saveRenewed(clientJwt string) {
	if err := Save(clientJwt); err != nil {
		logx.Warn("jwt renew: save failed: %v\n", err)
	} else if id := ParseClientID(clientJwt); id != "" {
		logx.Info("jwt renewed (client_id=%s)\n", id)
	} else {
		logx.Info("jwt renewed\n")
	}
}
