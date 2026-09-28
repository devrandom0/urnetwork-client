package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/docopt/docopt-go"
)

func cmdVpnFromOpts(ctx context.Context, opts docopt.Opts) error {
	cfg, err := resolveVPNConfig(opts)
	if err != nil {
		return err
	}
	jwt, err := loadJWT(getStringOr(opts, "--jwt", ""))
	if err != nil && vpnUsesTUN(cfg) {
		return fmt.Errorf("vpn needs a JWT (run 'login' or pass --jwt): %w", err)
	}
	cfg.JWT = jwt
	return cmdVpn(ctx, cfg)
}

// vpnUsesTUN mirrors the SOCKS-only detection in cmdVpn; SOCKS-only mode never calls the API.
func vpnUsesTUN(cfg VPNConfig) bool {
	tun := strings.TrimSpace(cfg.TunName)
	return tun != "" && !isTUNDisabled(tun)
}
