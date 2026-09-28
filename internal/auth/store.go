// Package auth manages the client JWT store and the login/renewal lifecycle.
package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/devrandom0/urnetwork-client/internal/safefile"
)

func Path() string {
	if base := strings.TrimSpace(os.Getenv("URNETWORK_HOME")); base != "" {
		return filepath.Join(base, "jwt")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".urnetwork", "jwt")
}

func Load(maybe string) (string, error) {
	if strings.TrimSpace(maybe) != "" {
		return strings.TrimSpace(maybe), nil
	}
	path := Path()
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no jwt provided and failed to read %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func Save(jwt string) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return safefile.WriteFile(path, []byte(strings.TrimSpace(jwt)+"\n"))
}

// ParseClientID extracts the client_id claim from a JWT without verifying its signature.
// The result is used for informational display and token-type detection only.
func ParseClientID(jwt string) string {
	claims := gojwt.MapClaims{}
	_, _, _ = gojwt.NewParser().ParseUnverified(jwt, claims)
	if v, ok := claims["client_id"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
