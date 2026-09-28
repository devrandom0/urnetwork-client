// Package urapi wraps the connect SDK behind a Go-native API for locations, auth and the VPN client.
package urapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/urnetwork/connect"
)

// newByAPI creates a BringYourApi client with an optional pre-set JWT.
// Pass an empty jwt for pre-auth calls such as Login.
func newByAPI(ctx context.Context, apiURL, jwt string) *connect.BringYourApi {
	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiURL)
	if strings.TrimSpace(jwt) != "" {
		api.SetByJwt(jwt)
	}
	return api
}

// LoginResult holds the outcome of a successful login attempt.
type LoginResult struct {
	ByJwt                string
	NetworkName          string
	VerificationRequired bool
}

// LoginWithPassword calls AuthLoginWithPassword synchronously and returns a LoginResult.
// If verification is required before a JWT can be issued, VerificationRequired is set and ByJwt is empty.
func LoginWithPassword(ctx context.Context, apiURL, userAuth, password string) (*LoginResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiURL, "")

	type outcome struct {
		lr  *LoginResult
		err error
	}
	ch := make(chan outcome, 1)
	api.AuthLoginWithPassword(
		&connect.AuthLoginWithPasswordArgs{UserAuth: userAuth, Password: password},
		connect.NewApiCallback(func(res *connect.AuthLoginWithPasswordResult, err error) {
			if err != nil {
				ch <- outcome{err: err}
				return
			}
			if res.Error != nil {
				ch <- outcome{err: fmt.Errorf("%s", res.Error.Message)}
				return
			}
			if res.VerificationRequired != nil {
				ch <- outcome{lr: &LoginResult{VerificationRequired: true}}
				return
			}
			if res.Network == nil || strings.TrimSpace(res.Network.ByJwt) == "" {
				ch <- outcome{err: errors.New("login succeeded but no by_jwt returned")}
				return
			}
			ch <- outcome{lr: &LoginResult{ByJwt: res.Network.ByJwt, NetworkName: res.Network.NetworkName}}
		}),
	)
	r := <-ch
	return r.lr, r.err
}

// VerifyCode calls AuthVerify synchronously and returns the network-scoped BY JWT.
func VerifyCode(ctx context.Context, apiURL, userAuth, code string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiURL, "")

	type outcome struct {
		byJwt string
		err   error
	}
	ch := make(chan outcome, 1)
	api.AuthVerify(
		&connect.AuthVerifyArgs{UserAuth: userAuth, VerifyCode: code},
		connect.NewApiCallback(func(res *connect.AuthVerifyResult, err error) {
			if err != nil {
				ch <- outcome{err: err}
				return
			}
			if res.Error != nil {
				ch <- outcome{err: fmt.Errorf("%s", res.Error.Message)}
				return
			}
			if res.Network == nil || strings.TrimSpace(res.Network.ByJwt) == "" {
				ch <- outcome{err: errors.New("verify succeeded but no by_jwt returned")}
				return
			}
			ch <- outcome{byJwt: res.Network.ByJwt}
		}),
	)
	r := <-ch
	return r.byJwt, r.err
}

// MintClientJWT exchanges any BY JWT (network- or client-scoped) for a fresh client-scoped JWT.
func MintClientJWT(ctx context.Context, apiURL, byJwt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiURL, byJwt)
	res, err := api.AuthNetworkClientSync(&connect.AuthNetworkClientArgs{Description: "", DeviceSpec: ""})
	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", fmt.Errorf("auth-client error: %s", res.Error.Message)
	}
	if strings.TrimSpace(res.ByClientJwt) == "" {
		return "", errors.New("auth-client succeeded but no by_client_jwt returned")
	}
	return res.ByClientJwt, nil
}

// ValidateClientJWT performs a lightweight authenticated API call to confirm the JWT is
// accepted by the backend. Returns true if the call succeeds.
func ValidateClientJWT(ctx context.Context, apiURL, clientJwt string) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiURL, clientJwt)
	specs := []*connect.ProviderSpec{{BestAvailable: true}}
	_, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs, Count: 1, RankMode: "quality"})
	return err == nil
}
