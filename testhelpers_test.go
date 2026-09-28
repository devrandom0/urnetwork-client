package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

// vpnTestOpts mimics what docopt returns for vpn/quick-connect, including its defaults.
func vpnTestOpts(overrides map[string]interface{}) docopt.Opts {
	o := docopt.Opts{
		"--api_url":             config.DefaultAPIURL,
		"--connect_url":         config.DefaultConnectURL,
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
