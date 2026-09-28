package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

func TestLoginRetryBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 10 * time.Second},
		{1, 20 * time.Second},
		{2, 40 * time.Second},
		{4, 160 * time.Second},
		{5, 5 * time.Minute},
		{50, 5 * time.Minute},
	}
	for _, tc := range cases {
		if got := loginRetryBackoff(tc.attempt); got != tc.want {
			t.Errorf("loginRetryBackoff(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func fakeClientJWT(t *testing.T, clientID string) string {
	t.Helper()
	tok, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{"client_id": clientID}).SignedString([]byte("test"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

func TestEnsureClientJWT_KeepsValidJWTFromFlag(t *testing.T) {
	t.Setenv("URNETWORK_HOME", t.TempDir())
	if err := Save(fakeClientJWT(t, "on-disk")); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/network/find-providers2" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"providers":[]}`))
	}))
	defer srv.Close()
	fromFlag := fakeClientJWT(t, "from-flag")

	got, err := EnsureClientJWT(context.Background(), srv.URL, fromFlag, false, "", "")
	if err != nil {
		t.Fatalf("EnsureClientJWT: %v", err)
	}
	if got != fromFlag {
		t.Fatalf("EnsureClientJWT returned client_id=%q; want the validated --jwt token, not the one on disk", ParseClientID(got))
	}
}
