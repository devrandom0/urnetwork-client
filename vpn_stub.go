//go:build !linux && !darwin

package main

import (
	"context"
	"errors"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

func cmdVpn(_ context.Context, _ config.VPNConfig) error {
	return errors.New("vpn is currently supported on Linux only (container) with --cap-add NET_ADMIN and /dev/net/tun")
}
