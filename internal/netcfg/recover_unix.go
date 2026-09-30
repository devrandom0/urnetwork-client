//go:build darwin || linux

package netcfg

import (
	"errors"
	"net"
)

var interfaceExists = func(name string) bool {
	_, err := net.InterfaceByName(name)
	return err == nil
}

// dnsMarker is one piece of evidence that a run pointed system DNS at its stub.
type dnsMarker struct {
	pid  int
	undo func() (string, error)
}

// checkOwner decides whether a marker left by pid may be undone under o.
func checkOwner(pid int, o RecoverOptions) error {
	if o.Owner > 0 {
		if pid != o.Owner {
			return ErrNotOwner
		}
		return nil
	}
	if o.Force {
		return nil
	}
	return liveOwnerErr(pid)
}

// recoverState checks every marker before changing anything, so a refusal leaves the host
// as it was. DNS goes first because a dead stub breaks every lookup, while leftover routes
// only block IPv6 or a gone TUN's prefixes.
func recoverState(o RecoverOptions, markers []dnsMarker, routeSteps ...func() ([]string, error)) ([]string, error) {
	for _, m := range markers {
		if err := checkOwner(m.pid, o); err != nil {
			return nil, err
		}
	}
	if o.Owner > 0 && len(markers) == 0 {
		return nil, ErrNotOwner
	}
	var actions []string
	var errs []error
	for _, m := range markers {
		a, err := m.undo()
		if a != "" {
			actions = append(actions, a)
		}
		errs = append(errs, err)
	}
	for _, step := range routeSteps {
		a, err := step()
		actions = append(actions, a...)
		errs = append(errs, err)
	}
	return actions, errors.Join(errs...)
}
