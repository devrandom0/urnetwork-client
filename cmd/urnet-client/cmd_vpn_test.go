package main

import (
	"context"
	"strings"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
)

func TestVpnUsesTUN(t *testing.T) {
	cases := map[string]bool{"": false, "none": false, "off": false, " NONE ": false, "utun9": true, "urnet0": true, "--default_route": true}
	for tun, want := range cases {
		if got := vpnUsesTUN(config.VPNConfig{TunName: tun}); got != want {
			t.Errorf("vpnUsesTUN(%q) = %v, want %v", tun, got, want)
		}
	}
}

func TestCmdVpnFromOpts_MissingJWTFailsInTUNMode(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	t.Setenv("URNETWORK_HOME", t.TempDir())
	err := cmdVpnFromOpts(context.Background(), vpnTestOpts(map[string]interface{}{"--tun": "urnet-test0"}))
	if err == nil || !strings.Contains(err.Error(), "JWT") {
		t.Fatalf("err = %v, want a JWT error before any TUN work", err)
	}
}

func TestCmdVpnFromOpts_SocksOnlyDoesNotNeedJWT(t *testing.T) {
	t.Cleanup(func() { logx.SetLogLevel("info", false) })
	t.Setenv("URNETWORK_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := cmdVpnFromOpts(ctx, vpnTestOpts(map[string]interface{}{"--tun": "none", "--socks": "127.0.0.1:0"}))
	if err != nil {
		t.Fatalf("SOCKS-only mode must not need a JWT: %v", err)
	}
}
