//go:build darwin

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

// Run (macOS): create a utun device and bridge packets with RemoteUserNatMultiClient.
// Note: You typically need sudo to create/configure utun and set routes.
func Run(ctx context.Context, cfg config.VPNConfig, appVersion string) error {
	tunName := cfg.TunName
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")

	logStartupConfig(cfg)

	if cfg.EnableKillSwitch && !cfg.DefaultRoute {
		logx.Warn("--kill_switch has no effect without --default_route; kill switch requires a full default route to block leaks\n")
	}

	// If TUN is disabled or not specified (and not a missing-arg case), run SOCKS-only.
	socksOnly := IsTUNDisabled(tunName) || (rawTun == "" && !tunLikelyMissingArg)
	if err := preflight(cfg, !socksOnly, netcfg.RecoverStaleDNS); err != nil {
		return err
	}
	if socksOnly {
		return runSocksOnly(ctx, cfg)
	}

	devName := tunName
	if tunLikelyMissingArg {
		devName = ""
		logx.Warn("--tun provided without a valid name (got %q); using auto utun\n", rawTun)
	}
	sd := newShutdown()
	var wd releaser
	defer func() { runShutdown(sd, wd) }()

	dev, err := tunnel.Open(devName)
	if err != nil {
		return err
	}
	sd.add("close TUN", closeTUNTimeout, dev.Close)
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
	pinned := endpointPins{}

	// Detect original default gateway before altering routes.
	defGw, _, gwErr := netcfg.DefaultGateway()
	if gwErr != nil && (cfg.DefaultRoute || strings.TrimSpace(cfg.ExcludeRoutes) != "") {
		logx.Warn("failed to detect default gateway: %v\n", gwErr)
	}

	// Set up route manager; its Cleanup runs during shutdown.
	rm := netcfg.NewDarwinRouteManager(actualName, peerIP, defGw)
	sd.add("remove routes", routeCleanupTimeout, func() error { rm.Cleanup(); return nil })

	// Install routes based on mode.
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

	if !cfg.DefaultRoute && cfg.DNSList != "" {
		rm.AddDNSServerRoutes(config.SplitCSV(cfg.DNSList), false)
	}

	var stubAddr string
	if cfg.DefaultRoute {
		wd = startDNSWatchdog()
		guard, err := applyLeakPolicy(ctx, cfg, pinned, &counters, leakDeps{
			routes:    rm,
			sysDNS:    netcfg.NewSystemDNS(actualName),
			startStub: startDNSStub,
			control:   socks.InterfaceControl(actualName),
		})
		if err != nil {
			wd = stayArmed{wd}
			return err
		}
		guard.register(sd)
		stubAddr = guard.StubAddr()
	}

	// Run shared dataplane + SOCKS + stats.
	runCore(ctx, dev, actualName, cfg, appVersion, &counters, stubAddr, sd)
	return nil
}
