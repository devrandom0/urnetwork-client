package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/docopt/docopt-go"
)

func testSecrets() []secretArg {
	return []secretArg{
		{Flag: "--password", Env: "URNETWORK_PASSWORD", Value: "hunter2"},
		{Flag: "--jwt", Env: "URNETWORK_JWT", Value: "eyJ.x.y"},
		{Flag: "--socks_pass", Env: "URNETWORK_SOCKS_PASS", Value: "sockspw"},
	}
}

func TestScrubSecretArgs(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{"equals form and background", []string{"urnet-client", "quick-connect", "--user_auth=me@x", "--password=hunter2", "--background"}, []string{"quick-connect", "--user_auth=me@x", "--password="}},
		{"separate value", []string{"urnet-client", "vpn", "--jwt", "eyJ.x.y", "--tun=urnet0"}, []string{"vpn", "--jwt=", "--tun=urnet0"}},
		{"docopt prefix abbreviation", []string{"urnet-client", "quick-connect", "--pass=hunter2"}, []string{"quick-connect", "--pass="}},
		{"--socks is not mistaken for --socks_pass", []string{"urnet-client", "vpn", "--socks=0.0.0.0:1080", "--socks_pass=sockspw", "--socks", "127.0.0.1:1081"}, []string{"vpn", "--socks=0.0.0.0:1080", "--socks_pass=", "--socks", "127.0.0.1:1081"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := scrubSecretArgs(tc.argv, testSecrets())
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScrubSecretArgs_EnvCarriesValues(t *testing.T) {
	_, env := scrubSecretArgs([]string{"urnet-client"}, testSecrets())
	sort.Strings(env)
	want := []string{"URNETWORK_JWT=eyJ.x.y", "URNETWORK_PASSWORD=hunter2", "URNETWORK_SOCKS_PASS=sockspw"}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("env = %q, want %q", env, want)
	}
}

func TestScrubSecretArgs_EmptySecretsChangeNothing(t *testing.T) {
	argv := []string{"urnet-client", "vpn", "--tun=urnet0", "--background"}
	args, env := scrubSecretArgs(argv, []secretArg{{Flag: "--jwt", Env: "URNETWORK_JWT"}})
	if !reflect.DeepEqual(args, []string{"vpn", "--tun=urnet0"}) || len(env) != 0 {
		t.Fatalf("args=%q env=%q", args, env)
	}
}

func TestApplySecretEnvFallbacks(t *testing.T) {
	opts := docopt.Opts{"--password": nil, "--jwt": "cli-jwt", "--socks_pass": nil}
	env := map[string]string{"URNETWORK_PASSWORD": "envpw", "URNETWORK_JWT": "env-jwt"}
	applySecretEnvFallbacks(opts, func(k string) string { return env[k] })
	if opts["--password"] != "envpw" {
		t.Fatalf("--password = %v, want envpw from env", opts["--password"])
	}
	if opts["--jwt"] != "cli-jwt" {
		t.Fatal("env must never override an explicit flag")
	}
	if opts["--socks_pass"] != nil {
		t.Fatalf("--socks_pass = %v, want untouched", opts["--socks_pass"])
	}
}

func TestBackgroundSecrets_ReadsParsedOptions(t *testing.T) {
	opts := docopt.Opts{"--password": "pw", "--jwt": nil}
	got := backgroundSecrets(opts)
	if len(got) != len(secretFlags) || got[0].Value != "pw" || got[1].Value != "" {
		t.Fatalf("got %+v", got)
	}
}
