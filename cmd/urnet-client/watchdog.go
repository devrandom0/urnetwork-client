package main

import (
	"errors"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
)

// watchdogRecovery runs the same recovery as the recover command, limited to the dead VPN's
// own leftovers: a marker naming another pid belongs to a newer run, or is already gone.
func watchdogRecovery(recover func(netcfg.RecoverOptions) ([]string, error)) func(owner int) error {
	return func(owner int) error {
		actions, err := recover(netcfg.RecoverOptions{Owner: owner})
		for _, a := range actions {
			logx.Info("watchdog: %s\n", a)
		}
		if errors.Is(err, netcfg.ErrNotOwner) {
			logx.Info("watchdog: system DNS is no longer set by pid %d; nothing to do\n", owner)
			return nil
		}
		return err
	}
}
