//go:build !linux && !darwin

package main

import (
	"context"
	"errors"
)

func cmdVpn(_ context.Context, _ VPNConfig) error {
	return errors.New("vpn is currently supported on Linux only (container) with --cap-add NET_ADMIN and /dev/net/tun")
}
