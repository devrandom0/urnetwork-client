package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/socks"
)

// vpnTestOpts mimics what docopt returns for vpn/quick-connect, including its defaults.
func vpnTestOpts(overrides map[string]interface{}) docopt.Opts {
	o := docopt.Opts{
		"--api_url":             DefaultAPIURL,
		"--connect_url":         DefaultConnectURL,
		"--ip_cidr":             "10.255.0.2/24",
		"--mtu":                 "1420",
		"--dns_bootstrap":       "bypass",
		"--stats_interval":      "5",
		"--default_route":       false,
		"--debug":               false,
		"--allow_inbound_local": false,
		"--enable_ipv6":         false,
		"--kill_switch":         false,
	}
	for k, v := range overrides {
		o[k] = v
	}
	return o
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveVPNConfig(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	cases := []struct {
		name  string
		file  string
		flags map[string]interface{}
		check func(t *testing.T, cfg VPNConfig)
	}{
		{"defaults without file", "", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.MTU != 1420 || cfg.TunName != "" || cfg.APIURL != DefaultAPIURL || cfg.JWT != "" {
				t.Fatalf("unexpected defaults: %+v", cfg)
			}
		}},
		{"file fills unset values", "tun: utun7\nmtu: 1380\nsocks: 127.0.0.1:1081\napi_url: https://api.example.test\n", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.TunName != "utun7" || cfg.MTU != 1380 || cfg.SOCKSListen != "127.0.0.1:1081" || cfg.APIURL != "https://api.example.test" {
				t.Fatalf("file values not applied: %+v", cfg)
			}
		}},
		{"CLI wins over file", "tun: utun7\n", map[string]interface{}{"--tun": "utun9"}, func(t *testing.T, cfg VPNConfig) {
			if cfg.TunName != "utun9" {
				t.Fatalf("tun = %q, want utun9", cfg.TunName)
			}
		}},
		{"socks_listen alias", "socks_listen: 127.0.0.1:1082\n", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.SOCKSListen != "127.0.0.1:1082" {
				t.Fatalf("socks_listen alias ignored: %q", cfg.SOCKSListen)
			}
		}},
		{"socks beats socks_listen", "socks: 127.0.0.1:1083\nsocks_listen: 127.0.0.1:1084\n", nil, func(t *testing.T, cfg VPNConfig) {
			if cfg.SOCKSListen != "127.0.0.1:1083" {
				t.Fatalf("socks = %q, want the canonical key to win", cfg.SOCKSListen)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := map[string]interface{}{}
			for k, v := range tc.flags {
				flags[k] = v
			}
			if tc.file != "" {
				flags["--config"] = writeTempConfig(t, tc.file)
			}
			cfg, err := resolveVPNConfig(vpnTestOpts(flags))
			if err != nil {
				t.Fatalf("resolveVPNConfig: %v", err)
			}
			tc.check(t, cfg)
		})
	}
}

func TestResolveVPNConfig_MissingFileIsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": missing})); err == nil {
		t.Fatal("want error for a missing config file")
	}
}

func TestResolveLogLevel(t *testing.T) {
	cases := []struct {
		name      string
		flagLevel string
		flagDebug bool
		fileLevel string
		fileDebug bool
		wantLevel string
		wantDebug bool
	}{
		{"nothing set", "", false, "", false, "", false},
		{"flag level wins over file", "warn", false, "debug", true, "warn", false},
		{"--debug flag wins over file level", "", true, "warn", false, "debug", true},
		{"file level used", "", false, "warn", false, "warn", false},
		{"file debug used", "", false, "", true, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lvl, dbg := resolveLogLevel(tc.flagLevel, tc.flagDebug, tc.fileLevel, tc.fileDebug)
			if lvl != tc.wantLevel || dbg != tc.wantDebug {
				t.Fatalf("got (%q, %v), want (%q, %v)", lvl, dbg, tc.wantLevel, tc.wantDebug)
			}
		})
	}
}

func TestResolveVPNConfig_RejectsCleartextURLFromFile(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	path := writeTempConfig(t, "api_url: http://api.example.test\n")
	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": path})); err == nil {
		t.Fatal("cleartext api_url from the config file must be rejected")
	}
	ok := writeTempConfig(t, "connect_url: ws://localhost:9000\n")
	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": ok})); err != nil {
		t.Fatalf("ws://localhost must be allowed: %v", err)
	}
}

func TestResolveVPNConfig_AppliesFileLogLevel(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	path := writeTempConfig(t, "log_level: warn\n")

	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": path})); err != nil {
		t.Fatal(err)
	}
	if logx.IsInfoEnabled() {
		t.Fatal("log_level: warn from the file was not applied")
	}

	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--config": path, "--log_level": "debug"})); err != nil {
		t.Fatal(err)
	}
	if !logx.IsDebugEnabled() {
		t.Fatal("--log_level must beat the file")
	}
}

func TestResolveVPNConfig_RejectsHalfConfiguredSocksAuth(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	t.Setenv("URNETWORK_SOCKS_USER", "")
	t.Setenv("URNETWORK_SOCKS_PASS", "")
	opts := vpnTestOpts(map[string]interface{}{"--tun": "urnet0", "--socks": "127.0.0.1:1080", "--socks_user": "alice"})
	if _, err := resolveVPNConfig(opts); err == nil {
		t.Fatal("resolveVPNConfig accepted --socks_user without a password; the proxy would start unauthenticated or not at all")
	}
}

func TestResolveVPNConfig_IgnoresSocksAuthWithoutSocks(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	t.Setenv("URNETWORK_SOCKS_USER", "alice")
	t.Setenv("URNETWORK_SOCKS_PASS", "")
	if _, err := resolveVPNConfig(vpnTestOpts(map[string]interface{}{"--tun": "urnet0"})); err != nil {
		t.Fatalf("resolveVPNConfig = %v; SOCKS auth only matters when --socks is set", err)
	}
}

func TestParseConfigs_SocksAuthFromFlagsAndEnv(t *testing.T) {
	t.Setenv("URNETWORK_SOCKS_USER", "")
	t.Setenv("URNETWORK_SOCKS_PASS", "from-env")
	opts := docopt.Opts{"--socks_user": "alice"}
	if got := parseVPNConfig(opts, "").SOCKSAuth; got != (socks.SocksAuth{User: "alice", Pass: "from-env"}) {
		t.Fatalf("vpn SOCKSAuth = %+v", got)
	}
	if got := parseSOCKSConfig(opts).Auth; got != (socks.SocksAuth{User: "alice", Pass: "from-env"}) {
		t.Fatalf("socks Auth = %+v", got)
	}
	if got := socksOptionsFromVPN(VPNConfig{SOCKSAuth: socks.SocksAuth{User: "a", Pass: "b"}}, "").Auth; got.User != "a" {
		t.Fatalf("socksOptionsFromVPN dropped auth: %+v", got)
	}
}
