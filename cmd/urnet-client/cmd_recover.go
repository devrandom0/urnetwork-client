package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
)

type recoverDeps struct {
	geteuid func() int
	recover func(netcfg.RecoverOptions) ([]string, error)
	out     io.Writer
}

func defaultRecoverDeps() recoverDeps {
	return recoverDeps{geteuid: os.Geteuid, recover: netcfg.Recover, out: os.Stdout}
}

// cmdRecover undoes the DNS and route changes of a VPN run that died without cleaning up.
func cmdRecover(opts docopt.Opts, d recoverDeps) error {
	if d.geteuid() != 0 {
		return errors.New("recover changes system DNS and routes and must run as root: sudo urnet-client recover")
	}
	actions, err := d.recover(netcfg.RecoverOptions{Force: config.MustBool(opts, "--force")})
	for _, a := range actions {
		_, _ = fmt.Fprintln(d.out, a)
	}
	var live *netcfg.LiveOwnerError
	if errors.As(err, &live) {
		return fmt.Errorf("%w; if pid %d is not a running urnet-client, run: sudo urnet-client recover --force", err, live.PID)
	}
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		_, _ = fmt.Fprintln(d.out, "nothing to recover")
	}
	return nil
}
