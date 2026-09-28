package main

import (
	"context"
	"strings"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

func TestCmdQuickConnect_LoadsConfigFile(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	t.Setenv("URNETWORK_HOME", t.TempDir())
	t.Setenv("URNETWORK_USERNAME", "")
	t.Setenv("URNETWORK_PASSWORD", "")
	bad := writeTempConfig(t, "mtu: [not-an-int\n")

	err := cmdQuickConnect(context.Background(), vpnTestOpts(map[string]interface{}{"--config": bad}), false)

	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("err = %v; quick-connect must load --config before doing anything else", err)
	}
}

func TestJWTLoadArgForStep2(t *testing.T) {
	cases := []struct {
		name           string
		jwtOpt         string
		jwtFromEnv     bool
		userAuth       string
		password       string
		wantLoadJWTArg string
	}{
		{"login just ran, env jwt is stale", "env-jwt", true, "me@x", "pw", ""},
		{"login just ran via password only, env jwt is stale", "env-jwt", true, "", "pw", ""},
		{"explicit --jwt wins over env login credentials", "cli-jwt", false, "me@x", "pw", "cli-jwt"},
		{"no login, explicit --jwt kept", "cli-jwt", false, "", "", "cli-jwt"},
		{"no login, env jwt kept", "env-jwt", true, "", "", "env-jwt"},
		{"no login, no jwt opt", "", false, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jwtLoadArgForStep2(tc.jwtOpt, tc.jwtFromEnv, tc.userAuth, tc.password)
			if got != tc.wantLoadJWTArg {
				t.Fatalf("jwtLoadArgForStep2(%q, %v, %q, %q) = %q, want %q", tc.jwtOpt, tc.jwtFromEnv, tc.userAuth, tc.password, got, tc.wantLoadJWTArg)
			}
		})
	}
}
