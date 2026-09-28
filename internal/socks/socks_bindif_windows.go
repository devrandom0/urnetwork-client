//go:build windows

package socks

import (
	"fmt"
	"strings"
)

// bindFDToInterface has no windows implementation; the VPN/route code that needs interface
// binding is unix-only, so a non-empty ifName here just reports the platform gap.
func bindFDToInterface(_ int, _, ifName string) error {
	if strings.TrimSpace(ifName) == "" {
		return nil
	}
	return fmt.Errorf("%w: %s: not supported on windows", errBindInterface, ifName)
}
