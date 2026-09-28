package main

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
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

func TestApplySecretEnvFallbacks_ReportsEnvSourcedFlags(t *testing.T) {
	opts := docopt.Opts{"--password": nil, "--jwt": "", "--socks_pass": "cli"}
	env := map[string]string{"URNETWORK_PASSWORD": "envpw", "URNETWORK_JWT": "env-jwt", "URNETWORK_SOCKS_PASS": "envsocks"}
	fromEnv := applySecretEnvFallbacks(opts, func(k string) string { return env[k] })
	if !fromEnv["--password"] {
		t.Fatal("--password came only from the environment and must be reported as such")
	}
	if fromEnv["--jwt"] || opts["--jwt"] != "env-jwt" {
		t.Fatalf("--jwt=%v fromEnv=%v; a redacted --jwt= placeholder on argv means the value came from the CLI", opts["--jwt"], fromEnv["--jwt"])
	}
	if fromEnv["--socks_pass"] {
		t.Fatal("--socks_pass was set on the CLI")
	}
}

func TestScrubSecretArgs_StripsBackgroundPrefixes(t *testing.T) {
	for _, flag := range []string{"--background", "--backg", "--back", "--ba", "--b"} {
		args, _ := scrubSecretArgs([]string{"urnet-client", "vpn", flag, "--tun=urnet0"}, nil)
		if !reflect.DeepEqual(args, []string{"vpn", "--tun=urnet0"}) {
			t.Errorf("%s: args = %q; a docopt prefix of --background would respawn forever", flag, args)
		}
	}
}

func envLookup(env []string) func(string) string {
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

func TestScrubSecretArgs_ChildArgvStillParses(t *testing.T) {
	cases := [][]string{
		{"urnet-client", "quick-connect", "--user_auth=u", "--password=p", "--back"},
		{"urnet-client", "vpn", "--socks_user=u", "--socks_pass=p", "--jwt=j", "--background"},
		{"urnet-client", "vpn", "--jwt", "j", "--socks=127.0.0.1:1080", "--background"},
	}
	for _, argv := range cases {
		t.Run(strings.Join(argv[1:], " "), func(t *testing.T) {
			parent, err := docopt.ParseArgs(usageText(), argv[1:], Version)
			if err != nil {
				t.Fatalf("parent parse: %v", err)
			}
			args, env := scrubSecretArgs(argv, backgroundSecrets(parent))
			child, err := docopt.ParseArgs(usageText(), args, Version)
			if err != nil {
				t.Fatalf("child argv %q does not parse: %v", args, err)
			}
			applySecretEnvFallbacks(child, envLookup(env))
			if bg, _ := child.Bool("--background"); bg {
				t.Fatalf("child argv %q still requests --background", args)
			}
			for _, s := range secretFlags {
				if got, want := config.StringOr(child, s.Flag, ""), config.StringOr(parent, s.Flag, ""); got != want {
					t.Errorf("%s = %q in the child, want %q", s.Flag, got, want)
				}
			}
		})
	}
}

func TestLoadSecretEnv_UnsetsSecretVarsAfterReading(t *testing.T) {
	t.Setenv("URNETWORK_PASSWORD", "envpw")
	t.Setenv("URNETWORK_JWT", "env-jwt")
	t.Setenv("URNETWORK_SOCKS_PASS", "envsocks")
	t.Setenv("URNETWORK_SOCKS_USER", "socksuser")
	opts := vpnTestOpts(map[string]interface{}{"--password": nil, "--jwt": nil, "--socks_pass": nil, "--socks_user": nil})

	fromEnv := loadSecretEnv(opts)

	for _, s := range secretFlags {
		if v, ok := os.LookupEnv(s.Env); ok {
			t.Errorf("%s=%q still in the environment; route and ifconfig children would inherit it", s.Env, v)
		}
		if !fromEnv[s.Flag] {
			t.Errorf("%s not reported as env-sourced", s.Flag)
		}
	}
	cfg := config.ParseVPNConfig(opts, "")
	if cfg.SOCKSAuth.User != "socksuser" || cfg.SOCKSAuth.Pass != "envsocks" {
		t.Fatalf("SOCKSAuth = %+v; the env values must be captured before they are unset", cfg.SOCKSAuth)
	}
	if config.StringOr(opts, "--password", "") != "envpw" || config.StringOr(opts, "--jwt", "") != "env-jwt" {
		t.Fatalf("opts lost env secrets: %v", opts)
	}
}

func TestStartBackground_ValidatesConfigBeforeSpawning(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	for name, body := range map[string]string{
		"bad yaml":          "mtu: [not-an-int\n",
		"cleartext api_url": "api_url: http://api.example.com\n",
	} {
		t.Run(name, func(t *testing.T) {
			opts := vpnTestOpts(map[string]interface{}{"vpn": true, "--config": writeTempConfig(t, body)})
			spawned := false
			_, err := startBackground(opts, []string{"urnet-client", "vpn", "--background"}, func([]string, []secretArg) (int, error) {
				spawned = true
				return 42, nil
			})
			if err == nil || spawned {
				t.Fatalf("err = %v, spawned = %v; want the parent to fail before starting a child that would die silently", err, spawned)
			}
		})
	}
}

func TestStartBackground_SpawnsWhenConfigValid(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	opts := vpnTestOpts(map[string]interface{}{"vpn": true, "--password": "pw"})
	var gotSecrets []secretArg
	pid, err := startBackground(opts, []string{"urnet-client", "vpn", "--background"}, func(_ []string, s []secretArg) (int, error) {
		gotSecrets = s
		return 42, nil
	})
	if err != nil || pid != 42 {
		t.Fatalf("pid, err = %d, %v", pid, err)
	}
	if len(gotSecrets) == 0 || gotSecrets[0].Value != "pw" {
		t.Fatalf("secrets = %+v; want parsed secret values handed to the child", gotSecrets)
	}
}
