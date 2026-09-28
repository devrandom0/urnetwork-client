// Package session runs the VPN dataplane and SOCKS lifecycle for a connected session.
package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/socks"
	"github.com/devrandom0/urnetwork-client/internal/tunnel"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

// runCore selects providers, runs the dataplane between dev and the provider, prints stats
// and serves the optional SOCKS proxy until ctx ends, then runs onBeforeExit.
func runCore(
	ctx context.Context,
	dev tunnel.Device,
	tunIfName string,
	cfg config.VPNConfig,
	appVersion string,
	counters *tunnel.Counters,
	onBeforeExit func(),
) {
	gen := urapi.NewGenerator(ctx, urapi.GeneratorConfig{
		APIURL:     cfg.APIURL,
		ConnectURL: cfg.ConnectURL,
		JWT:        cfg.JWT,
		AppVersion: appVersion,
		Location:   cfg.Location,
	})
	policy := tunnel.NewInboundPolicy(cfg.AllowInboundSrcList, cfg.AllowInboundLocal, cfg.IPCIDR)
	dp := tunnel.NewDataplane(dev, policy, cfg.EnableIPv6, counters)
	mc := urapi.NewMultiClient(ctx, gen, dp.Receive)
	go dp.PumpOutbound(mc.SendPacket)

	if cfg.StatsInterval > 0 && logx.IsInfoEnabled() {
		go tunnel.LogStats(ctx, cfg.StatsInterval, counters)
	}

	// Optional SOCKS5 proxy bound to the VPN interface
	var stopSocks func() error
	if cfg.SOCKSListen != "" {
		warnIfSocksDNSUnset(tunIfName, cfg.DNSList)
		if s, err := socks.StartSocks5(ctx, socksOptionsFromVPN(cfg, tunIfName)); err != nil {
			logx.Warn("failed to start socks at %s: %v\n", cfg.SOCKSListen, err)
		} else {
			stopSocks = s
			logx.Info("SOCKS5 listening at %s (bound to %s)\n", cfg.SOCKSListen, tunIfName)
		}
	}

	if logx.IsInfoEnabled() {
		fmt.Println("VPN dataplane running; press Ctrl-C to exit.")
	}

	// Wait for termination via context cancellation
	<-ctx.Done()

	// Cleanup order: stop socks, then OS-specific cleanup
	if stopSocks != nil {
		_ = stopSocks()
	}
	if onBeforeExit != nil {
		onBeforeExit()
	}
}

// warnIfSocksDNSUnset logs once at startup when SOCKS hostname lookups will be resolved
// through the VPN interface with no explicit --dns override: a LAN or Docker (127.0.0.11)
// resolver is unreachable from inside the tunnel, so lookups fail closed silently otherwise.
func warnIfSocksDNSUnset(bindIf string, dnsList string) {
	if bindIf == "" || strings.TrimSpace(dnsList) != "" {
		return
	}
	logx.Warn("SOCKS hostname lookups go through the VPN interface and fail closed; if the system resolver is on the LAN or is Docker's 127.0.0.11, set --dns=<public resolver> (e.g. 1.1.1.1)\n")
}

func socksOptionsFromVPN(cfg config.VPNConfig, bindIf string) socks.SocksOptions {
	return socks.SocksOptions{
		ListenAddr:     cfg.SOCKSListen,
		BindIf:         bindIf,
		Auth:           cfg.SOCKSAuth,
		Debug:          cfg.Debug || logx.IsDebugEnabled(),
		AllowDomains:   cfg.AllowDomains,
		ExcludeDomains: cfg.ExcludeDomains,
		DNSServers:     config.SplitCSV(cfg.DNSList),
	}
}

// runSocksOnly serves SOCKS with system routing when no TUN is configured.
func runSocksOnly(ctx context.Context, cfg config.VPNConfig) error {
	if cfg.SOCKSListen == "" {
		return errors.New("no TUN and no --socks given; nothing to do (set --tun=<name> and/or --socks=<addr>)")
	}
	stop, err := socks.StartSocks5(ctx, socksOptionsFromVPN(cfg, ""))
	if err != nil {
		return fmt.Errorf("start socks failed: %w", err)
	}
	defer func() { _ = stop() }()
	logx.Info("SOCKS started without TUN (system routes only). Press Ctrl+C to exit.\n")
	<-ctx.Done()
	return nil
}

// IsTUNDisabled returns true when the provided name represents a disabled TUN
// (e.g., "none", "no", "off", "false", "disable", "0").
func IsTUNDisabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "none", "non", "no", "off", "false", "disable", "disabled", "0":
		return true
	default:
		return false
	}
}

// logStartupConfig logs the effective VPN configuration summary from a config.VPNConfig.
func logStartupConfig(cfg config.VPNConfig) {
	configItems := []string{
		fmt.Sprintf("api_url=%s", cfg.APIURL),
		fmt.Sprintf("connect_url=%s", cfg.ConnectURL),
		fmt.Sprintf("tun=%s", cfg.TunName),
		fmt.Sprintf("ip_cidr=%s", cfg.IPCIDR),
		fmt.Sprintf("mtu=%d", cfg.MTU),
		fmt.Sprintf("default_route=%t", cfg.DefaultRoute),
	}
	if strings.TrimSpace(cfg.ExtraRoutes) != "" {
		configItems = append(configItems, fmt.Sprintf("route=%s", strings.TrimSpace(cfg.ExtraRoutes)))
	}
	if strings.TrimSpace(cfg.ExcludeRoutes) != "" {
		configItems = append(configItems, fmt.Sprintf("exclude_route=%s", strings.TrimSpace(cfg.ExcludeRoutes)))
	}
	if strings.TrimSpace(cfg.DNSList) != "" {
		configItems = append(configItems, fmt.Sprintf("dns=%s", strings.TrimSpace(cfg.DNSList)))
	}
	if cfg.DNSService != "" {
		configItems = append(configItems, fmt.Sprintf("dns_service=%s", cfg.DNSService))
	}
	if cfg.DNSBootstrap != "" {
		configItems = append(configItems, fmt.Sprintf("dns_bootstrap=%s", cfg.DNSBootstrap))
	}
	if cfg.SOCKSListen != "" {
		configItems = append(configItems, fmt.Sprintf("socks=%s", cfg.SOCKSListen))
	}
	if len(cfg.AllowDomains) > 0 {
		configItems = append(configItems, fmt.Sprintf("domain=%s", strings.Join(cfg.AllowDomains, ",")))
	}
	if len(cfg.ExcludeDomains) > 0 {
		configItems = append(configItems, fmt.Sprintf("exclude_domain=%s", strings.Join(cfg.ExcludeDomains, ",")))
	}
	if cfg.EnableKillSwitch {
		configItems = append(configItems, "kill_switch=enabled")
	}
	if cfg.EnableIPv6 {
		configItems = append(configItems, "ipv6=enabled")
	}
	configItems = append(configItems, fmt.Sprintf("debug=%t", cfg.Debug))
	if strings.TrimSpace(cfg.JWT) != "" {
		configItems = append(configItems, "jwt=provided")
	} else {
		configItems = append(configItems, "jwt=missing")
	}
	logx.Info("startup: %s\n", strings.Join(configItems, " "))
}

// (removed legacy userspace filtering helpers)
