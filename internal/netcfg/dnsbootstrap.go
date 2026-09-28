//go:build darwin

package netcfg

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

type dnsBypassRemover interface{ RemoveDNSBypass() }

// RemoveDNSBypassWhenWarm drops the DNS bypass routes once the tunnel carries traffic
// in both directions, or at maxWait. It returns without touching routes if ctx ends first.
func RemoveDNSBypassWhenWarm(ctx context.Context, rm dnsBypassRemover, pktsIn, pktsOut *uint64, maxWait, tick time.Duration) {
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
		case <-ticker.C:
			if atomic.LoadUint64(pktsIn) == 0 || atomic.LoadUint64(pktsOut) == 0 {
				continue
			}
		}
		rm.RemoveDNSBypass()
		logx.Info("DNS bootstrap cache complete; DNS bypass removed\n")
		return
	}
}
