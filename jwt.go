package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
	"github.com/urnetwork/connect"
)

func jwtPath() string {
	if base := strings.TrimSpace(os.Getenv("URNETWORK_HOME")); base != "" {
		return filepath.Join(base, "jwt")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".urnetwork", "jwt")
}

func loadJWT(maybe string) (string, error) {
	if strings.TrimSpace(maybe) != "" {
		return strings.TrimSpace(maybe), nil
	}
	path := jwtPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no jwt provided and failed to read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func saveJWT(jwt string) error {
	path := jwtPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(jwt)+"\n"), 0o600)
}

// parseClientID extracts the client_id claim from a JWT without verifying its signature.
// The result is used for informational display and token-type detection only.
func parseClientID(jwt string) string {
	claims := gojwt.MapClaims{}
	_, _, _ = gojwt.NewParser().ParseUnverified(jwt, claims)
	if v, ok := claims["client_id"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// validateClientJWT performs a lightweight authenticated API call to confirm the JWT is
// accepted by the backend. Returns true if the call succeeds.
func validateClientJWT(ctx context.Context, apiUrl, clientJwt string) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	api := newByAPI(ctx, apiUrl, clientJwt)
	specs := []*connect.ProviderSpec{{BestAvailable: true}}
	_, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs, Count: 1, RankMode: "quality"})
	return err == nil
}
