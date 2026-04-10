package main

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
func newByAPI(ctx context.Context, apiUrl, jwt string) *connect.BringYourApi {
	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
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

// loginWithPassword calls AuthLoginWithPassword synchronously and returns a LoginResult.
// If verification is required before a JWT can be issued, VerificationRequired is set and ByJwt is empty.
func loginWithPassword(ctx context.Context, apiUrl, userAuth, password string) (*LoginResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiUrl, "")

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

// verifyCode calls AuthVerify synchronously and returns the network-scoped BY JWT.
func verifyCode(ctx context.Context, apiUrl, userAuth, code string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiUrl, "")

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

// mintClientJWT exchanges any BY JWT (network- or client-scoped) for a fresh client-scoped JWT.
func mintClientJWT(ctx context.Context, apiUrl, byJwt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiUrl, byJwt)
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
