//go:build linux

package session

import (
	"context"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
	"github.com/devrandom0/urnetwork-client/internal/socks"
	"github.com/devrandom0/urnetwork-client/internal/tunnel"
)

func Run(ctx context.Context, cfg config.VPNConfig, appVersion string) error {
	tunName := cfg.TunName
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")

	logStartupConfig(cfg)

	if cfg.EnableKillSwitch && !cfg.DefaultRoute {
		logx.Warn("--kill_switch has no effect without --default_route; kill switch requires a full default route to block leaks\n")
	}

	// TUN-less mode: SOCKS-only when TUN is disabled or not specified.
	socksOnly := IsTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg)
	if err := preflight(cfg, !socksOnly, netcfg.RecoverStaleDNS); err != nil {
		return err
	}
	if socksOnly {
		return runSocksOnly(ctx, cfg)
	}

	// If tun name looks like a flag (missing value), use a safe default.
	if tunLikelyMissingArg {
		logx.Warn("--tun provided without a valid name (got %q); using default 'urnet0'\n", rawTun)
		tunName = "urnet0"
	}

	dev, err := tunnel.Open(tunName)
	if err != nil {
		return err
	}
	defer func() { _ = dev.Close() }()
	logx.Info("TUN %s created\n", tunName)

	if err := netcfg.ConfigureLinuxTUN(tunName, cfg.IPCIDR, cfg.MTU, cfg.EnableIPv6); err != nil {
		return err
	}

	// Detect current default gateway for bypass and exclude routing.
	origGw, origDev := "", ""
	if routes, err := netcfg.ListDefaultRoutes(); err == nil {
		for _, r := range routes {
			if r.Dev == tunName {
				continue
			}
			// Prefer the route that has a gateway.
			if origDev == "" || (origGw == "" && r.Gw != "") {
				origGw, origDev = r.Gw, r.Dev
			}
		}
	}

	// Set up route manager; Cleanup runs on exit via defer.
	rm := netcfg.NewLinuxRouteManager(tunName, origGw, origDev)
	defer rm.Cleanup()

	// Install routes.
	var counters tunnel.Counters
	pinned := endpointPins{}
	if cfg.DefaultRoute {
		if cfg.EnableKillSwitch {
			if err := rm.AddKillSwitchRoute(); err != nil {
				return err
			}
		}
		pinned.add(rm.AddBypassEndpoint(cfg.APIURL))
		pinned.add(rm.AddBypassEndpoint(cfg.ConnectURL))
		if err := rm.AddSplitDefault(); err != nil {
			return err
		}
		for _, r := range config.SplitCSV(cfg.ExcludeRoutes) {
			rm.AddExclude(r)
		}
	}
	for _, r := range config.SplitCSV(cfg.ExtraRoutes) {
		rm.AddExtraRoute(r)
	}
	if !cfg.DefaultRoute && cfg.DNSList != "" {
		rm.AddDNSServerRoutes(config.SplitCSV(cfg.DNSList), false)
	}

	var stubAddr string
	if cfg.DefaultRoute {
		guard, err := applyLeakPolicy(ctx, cfg, pinned, &counters, leakDeps{
			routes:        rm,
			sysDNS:        netcfg.NewSystemDNS(tunName),
			startStub:     startDNSStub,
			control:       socks.InterfaceControl(tunName),
			ipv6Available: netcfg.IPv6Available,
		})
		if err != nil {
			return err
		}
		defer guard.Close()
		stubAddr = guard.StubAddr()
	}

	// Run shared dataplane + SOCKS + stats.
	runCore(ctx, dev, tunName, cfg, appVersion, &counters, stubAddr, func() {})
	return nil
}
