//go:build !darwin && !linux

package netcfg

import "errors"

// Recover is not available where the VPN itself does not run.
func Recover(RecoverOptions) ([]string, error) {
	return nil, errors.New("recover is supported on macOS and Linux only")
}
