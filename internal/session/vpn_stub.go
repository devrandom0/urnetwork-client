//go:build !linux && !darwin

package session

import (
	"context"
	"errors"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

func Run(_ context.Context, _ config.VPNConfig, _ string) error {
	return errors.New("vpn is currently supported on Linux only (container) with --cap-add NET_ADMIN and /dev/net/tun")
}
