package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/auth"
	"github.com/devrandom0/urnetwork-client/internal/config"
)

func cmdVpnFromOpts(ctx context.Context, opts docopt.Opts) error {
	cfg, err := config.ResolveVPNConfig(opts)
	if err != nil {
		return err
	}
	jwt, err := auth.Load(config.StringOr(opts, "--jwt", ""))
	if err != nil && vpnUsesTUN(cfg) {
		return fmt.Errorf("vpn needs a JWT (run 'login' or pass --jwt): %w", err)
	}
	cfg.JWT = jwt
	return cmdVpn(ctx, cfg)
}

// vpnUsesTUN mirrors the SOCKS-only detection in cmdVpn; SOCKS-only mode never calls the API.
func vpnUsesTUN(cfg config.VPNConfig) bool {
	tun := strings.TrimSpace(cfg.TunName)
	return tun != "" && !isTUNDisabled(tun)
}
