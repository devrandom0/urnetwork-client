package main

import (
	"context"
	"strings"
	"testing"
	"time"
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

func TestCmdQuickConnect_LoadsConfigFile(t *testing.T) {
	t.Cleanup(func() { setLogLevel("info", false) })
	t.Setenv("URNETWORK_HOME", t.TempDir())
	t.Setenv("URNETWORK_USERNAME", "")
	t.Setenv("URNETWORK_PASSWORD", "")
	bad := writeTempConfig(t, "mtu: [not-an-int\n")

	err := cmdQuickConnect(context.Background(), vpnTestOpts(map[string]interface{}{"--config": bad}))

	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("err = %v; quick-connect must load --config before doing anything else", err)
	}
}

func TestJWTLoadArgForStep2(t *testing.T) {
	cases := []struct {
		name           string
		jwtOpt         string
		userAuth       string
		password       string
		wantLoadJWTArg string
	}{
		{"login just ran via user_auth, ignores stale jwt opt", "env-jwt", "me@x", "pw", ""},
		{"login just ran via password only", "env-jwt", "", "pw", ""},
		{"no login, explicit --jwt kept", "cli-jwt", "", "", "cli-jwt"},
		{"no login, no jwt opt", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jwtLoadArgForStep2(tc.jwtOpt, tc.userAuth, tc.password)
			if got != tc.wantLoadJWTArg {
				t.Fatalf("jwtLoadArgForStep2(%q, %q, %q) = %q, want %q", tc.jwtOpt, tc.userAuth, tc.password, got, tc.wantLoadJWTArg)
			}
		})
	}
}
