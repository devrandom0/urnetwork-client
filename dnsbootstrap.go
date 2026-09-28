//go:build darwin

package main

import (
	"context"
	"sync/atomic"
	"time"
)

type dnsBypassRemover interface{ RemoveDNSBypass() }

// removeDNSBypassWhenWarm drops the DNS bypass routes once the tunnel carries traffic
// in both directions, or at maxWait. It returns without touching routes if ctx ends first.
func removeDNSBypassWhenWarm(ctx context.Context, rm dnsBypassRemover, pktsIn, pktsOut *uint64, maxWait, tick time.Duration) {
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
		logInfo("DNS bootstrap cache complete; DNS bypass removed\n")
		return
	}
}
