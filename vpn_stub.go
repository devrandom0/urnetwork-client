//go:build !linux && !darwin

package main

import (
	"errors"
	"github.com/docopt/docopt-go"
)

func cmdVpn(opts docopt.Opts) {
	fatal(errors.New("vpn is currently supported on Linux only (container) with --cap-add NET_ADMIN and /dev/net/tun"))
}
