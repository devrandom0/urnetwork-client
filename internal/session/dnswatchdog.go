//go:build darwin || linux

package session

import (
	"os"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
	"github.com/devrandom0/urnetwork-client/internal/watchdog"
)

// startDNSWatchdog runs before system DNS changes, so there is no moment where DNS points at
// the stub and nothing would restore it if this process were killed. A watchdog that fails
// to start only costs that safety net, so the VPN runs on with a warning.
func startDNSWatchdog() releaser {
	h, err := watchdog.Start(os.Getpid())
	if err != nil {
		logx.Warn("DNS watchdog not started: %v; if this process is killed, %s\n", err, netcfg.RecoverHint)
		return nil
	}
	return h
}
