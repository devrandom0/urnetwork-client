//go:build darwin

package main

import (
	"context"
	"strings"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/netcfg"
	"github.com/devrandom0/urnetwork-client/internal/tunnel"
)

// cmdVpn (macOS): create a utun device and bridge packets with RemoteUserNatMultiClient.
// Note: You typically need sudo to create/configure utun and set routes.
func cmdVpn(ctx context.Context, cfg config.VPNConfig) error {
	tunName := cfg.TunName
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")

	logStartupConfig(cfg)

	if cfg.EnableKillSwitch && !cfg.DefaultRoute {
		logx.Warn("--kill_switch has no effect without --default_route; kill switch requires a full default route to block leaks\n")
	}

	// If TUN is disabled or not specified (and not a missing-arg case), run SOCKS-only.
	if isTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		return runSocksOnly(ctx, cfg)
	}

	devName := tunName
	if tunLikelyMissingArg {
		devName = ""
		logx.Warn("--tun provided without a valid name (got %q); using auto utun\n", rawTun)
	}
	dev, err := tunnel.Open(devName)
	if err != nil {
		return err
	}
	defer func() { _ = dev.Close() }()
	actualName := dev.Name()
	if actualName == "" {
		actualName = tunName
	}
	logx.Info("TUN %s created\n", actualName)

	peerIP, err := netcfg.ConfigureDarwinTUN(actualName, cfg.IPCIDR, cfg.MTU, cfg.EnableIPv6)
	if err != nil {
		return err
	}

	if cfg.SOCKSListen != "" && !cfg.DefaultRoute && cfg.ExtraRoutes == "" && cfg.ExcludeRoutes == "" {
		logx.Info("SOCKS mode without route changes: only SOCKS traffic will use the VPN.\n")
	}

	// Packet counters (shared with the DNS cache goroutine).
	var counters tunnel.Counters

	// Detect original default gateway before altering routes.
	defGw, _, gwErr := netcfg.DefaultGateway()
	if gwErr != nil && (cfg.DefaultRoute || strings.TrimSpace(cfg.ExcludeRoutes) != "") {
		logx.Warn("failed to detect default gateway: %v\n", gwErr)
	}

	// Set up route manager; Cleanup runs on exit via defer.
	rm := netcfg.NewDarwinRouteManager(actualName, peerIP, defGw)
	defer rm.Cleanup()

	// Install routes based on mode.
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
		for _, r := range config.SplitCSV(cfg.ExcludeRoutes) {
			rm.AddExclude(r)
		}
	} else if cfg.SOCKSListen != "" {
		if cfg.ExtraRoutes == "" && cfg.ExcludeRoutes == "" {
			rm.AddScopedDefault()
		}
		for _, r := range config.SplitCSV(cfg.ExcludeRoutes) {
			rm.AddScopedExclude(r)
		}
	}
	for _, r := range config.SplitCSV(cfg.ExtraRoutes) {
		rm.AddExtraRoute(r)
	}

	// DNS configuration.
	if cfg.DNSList != "" {
		if cfg.DNSService != "" {
			if err := rm.SetDNS(config.SplitCSV(cfg.DNSList), cfg.DNSService); err != nil {
				logx.Warn("failed to set DNS via networksetup for %s: %v\n", cfg.DNSService, err)
			}
		} else {
			logx.Warn("--dns provided without --dns_service; skipping DNS change on macOS\n")
		}
		bypass := cfg.DefaultRoute && (cfg.DNSBootstrap == "bypass" || cfg.DNSBootstrap == "cache")
		rm.AddDNSServerRoutes(config.SplitCSV(cfg.DNSList), bypass)
	} else if cfg.DefaultRoute && defGw != "" && (cfg.DNSBootstrap == "bypass" || cfg.DNSBootstrap == "cache") {
		// No --dns: bypass current system resolvers so DNS works during default-route switch.
		if resolvers, err := netcfg.SystemDNSResolvers(); err == nil {
			rm.AddDNSServerRoutes(resolvers, true)
			if len(resolvers) > 0 {
				logx.Info("Kept existing DNS resolvers via %s: %v\n", defGw, resolvers)
			}
		} else {
			logx.Warn("failed to detect system DNS resolvers: %v\n", err)
		}
	}
	if !cfg.DefaultRoute && cfg.DNSList != "" {
		rm.AddDNSServerRoutes(config.SplitCSV(cfg.DNSList), false)
	}

	// DNS cache bootstrap: remove DNS bypass once the tunnel has traffic.
	if cfg.DefaultRoute && cfg.DNSBootstrap == "cache" {
		go netcfg.RemoveDNSBypassWhenWarm(ctx, rm, &counters.PktsIn, &counters.PktsOut, 3*time.Second, 200*time.Millisecond)
	}

	// Run shared dataplane + SOCKS + stats.
	vpnRunCore(ctx, dev, actualName, cfg, &counters, func() {})
	return nil
}
