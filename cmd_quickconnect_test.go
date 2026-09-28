package main

import "testing"

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
