//go:build linux

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/songgao/water"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
)

func cmdVpn(ctx context.Context, cfg VPNConfig) error {
	tunName := cfg.TunName
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")

	logStartupConfig(cfg)

	if cfg.EnableKillSwitch && !cfg.DefaultRoute {
		logx.Warn("--kill_switch has no effect without --default_route; kill switch requires a full default route to block leaks\n")
	}

	// TUN-less mode: SOCKS-only when TUN is disabled or not specified.
	if isTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		return runSocksOnly(ctx, cfg)
	}

	// If tun name looks like a flag (missing value), use a safe default.
	if tunLikelyMissingArg {
		logx.Warn("--tun provided without a valid name (got %q); using default 'urnet0'\n", rawTun)
		tunName = "urnet0"
	}

	// Create TUN device.
	waterCfg := water.Config{DeviceType: water.TUN}
	waterCfg.Name = tunName
	dev, err := water.New(waterCfg)
	if err != nil {
		return fmt.Errorf("create TUN failed: %w", err)
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
	if cfg.DefaultRoute {
		if cfg.EnableKillSwitch {
			if err := rm.AddKillSwitchRoute(); err != nil {
				return err
			}
		}
		rm.AddBypassEndpoint(cfg.APIURL)
		rm.AddBypassEndpoint(cfg.ConnectURL)
		if err := rm.AddSplitDefault(); err != nil {
			return err
		}
		for _, r := range splitCSV(cfg.ExcludeRoutes) {
			rm.AddExclude(r)
		}
	}
	for _, r := range splitCSV(cfg.ExtraRoutes) {
		rm.AddExtraRoute(r)
	}
	if !cfg.DefaultRoute && cfg.DNSList != "" {
		rm.AddDNSServerRoutes(splitCSV(cfg.DNSList), false)
	}

	// Run shared dataplane + SOCKS + stats.
	var pktsIn, bytesIn, pktsOut, bytesOut uint64
	vpnRunCore(ctx, dev, tunName, cfg, &pktsIn, &pktsOut, &bytesIn, &bytesOut, func() {})
	return nil
}
