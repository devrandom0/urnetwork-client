//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/songgao/water"

	"github.com/urnetwork/connect"
	"github.com/urnetwork/connect/protocol"
)

func cmdVpn(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
	// Default: no TUN unless explicitly provided
	tunName := getStringOr(opts, "--tun", "")
	rawTun := strings.TrimSpace(tunName)
	tunLikelyMissingArg := rawTun != "" && strings.HasPrefix(rawTun, "-")
	ipCIDR := getStringOr(opts, "--ip_cidr", "10.255.0.2/24")
	mtu := getIntOr(opts, "--mtu", 1420)
	defRoute, _ := opts.Bool("--default_route")
	extraRoutes := getStringOr(opts, "--route", "")
	excludeRoutes := getStringOr(opts, "--exclude_route", "")
	dnsList := getStringOr(opts, "--dns", "")
	debugOn, _ := opts.Bool("--debug")
	socksListen := strings.TrimSpace(getStringOr(opts, "--socks", getStringOr(opts, "--socks_listen", "")))
	allowDomains := splitCSV(getStringOr(opts, "--domain", ""))
	excludeDomains := splitCSV(getStringOr(opts, "--exclude_domain", ""))
	statsInt := time.Duration(getIntOr(opts, "--stats_interval", 5)) * time.Second
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TUN-less mode for SOCKS-only usage (none/non/no/off/false/disable/0 or empty and not a missing-arg case)
	if func(n string) bool {
		s := strings.ToLower(strings.TrimSpace(n))
		switch s {
		case "none", "non", "no", "off", "false", "disable", "disabled", "0":
			return true
		default:
			return false
		}
	}(tunName) || (rawTun == "" && !tunLikelyMissingArg) {
		if socksListen == "" {
			logError("--tun=none specified but no --socks provided; nothing to do\n")
			return
		}
		stopSocks, err := StartSocks5(ctx, socksListen, "", debugOn, allowDomains, excludeDomains)
		if err != nil {
			fatal(fmt.Errorf("start socks failed: %w", err))
		}
		defer stopSocks()
		logInfo("SOCKS started without TUN (system routes only). Press Ctrl+C to exit.\n")
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
		<-sigc
		return
	}

	// If tun name looks like a flag (missing value), choose a sane default name
	if tunLikelyMissingArg {
		logWarn("--tun provided without a valid name (got %q); using default 'urnet0'\n", rawTun)
		tunName = "urnet0"
	}

	cfg := water.Config{DeviceType: water.TUN}
	cfg.Name = tunName
	dev, err := water.New(cfg)
	if err != nil {
		fatal(fmt.Errorf("create TUN failed: %w", err))
	}
	defer dev.Close()
	logInfo("TUN %s created (configuring IP/MTU/routes as requested)\n", tunName)

	// Configure IP and MTU
	_ = run("ip", "addr", "add", ipCIDR, "dev", tunName)
	_ = run("ip", "link", "set", "dev", tunName, "mtu", fmt.Sprintf("%d", mtu))
	_ = run("ip", "link", "set", tunName, "up")

	// Routes: default or extra
	var origGw, origDev string
	var origMetric int = -1
	if defRoute {
		// Add a lower-metric default via the TUN, and bump current default route metric higher.
		// 1) Detect existing default route
		if gw, dev, met, err := linuxGetDefaultRoute(); err == nil {
			origGw, origDev, origMetric = gw, dev, met
		}
		// 2) Add TUN default with metric 50 (fallback to replace if exists)
		if err := run("ip", "route", "add", "default", "dev", tunName, "metric", "50"); err != nil {
			_ = run("ip", "route", "replace", "default", "dev", tunName, "metric", "50")
		}
		// 3) Bump original default metric to 200 so TUN takes precedence
		if origDev != "" {
			var err error
			if origGw != "" {
				// Route via a gateway
				err = run("ip", "route", "change", "default", "via", origGw, "dev", origDev, "metric", "200")
				if err != nil {
					_ = run("ip", "route", "replace", "default", "via", origGw, "dev", origDev, "metric", "200")
				}
			} else {
				// Link-scope default route (no gateway)
				err = run("ip", "route", "change", "default", "dev", origDev, "metric", "200")
				if err != nil {
					_ = run("ip", "route", "replace", "default", "dev", origDev, "metric", "200")
				}
			}
		}
		// 4) Excludes: prefer unreachable so they avoid default
		if strings.TrimSpace(excludeRoutes) != "" {
			for _, r := range strings.Split(excludeRoutes, ",") {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				_ = run("ip", "route", "add", r, "unreachable")
			}
		}
	}
	if strings.TrimSpace(extraRoutes) != "" {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			_ = run("ip", "route", "add", r, "dev", tunName)
		}
	}
	if !defRoute && strings.TrimSpace(dnsList) != "" {
		for _, d := range strings.Split(dnsList, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			_ = run("ip", "route", "add", d, "dev", tunName)
		}
	}

	strat := connect.NewClientStrategyWithDefaults(ctx)
	// Build provider specs based on location flags
	specs := []*connect.ProviderSpec{}
	if id := strings.TrimSpace(getStringOr(opts, "--location_id", "")); id != "" {
		if loc, err := connect.ParseId(id); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationId: &loc})
		}
	}
	if gid := strings.TrimSpace(getStringOr(opts, "--location_group_id", "")); gid != "" {
		if lg, err := connect.ParseId(gid); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationGroupId: &lg})
		}
	}
	if len(specs) == 0 {
		if q := strings.TrimSpace(getStringOr(opts, "--location_query", "")); q != "" {
			if httpRes, err := httpFindLocations(ctx, apiUrl, jwt, q); err == nil && httpRes != nil && len(httpRes.Specs) > 0 {
				specs = httpRes.Specs
				fmt.Printf("using %d specs from location query: %s\n", len(specs), q)
			}
			if len(specs) == 0 {
				fb := findSpecsByQueryFallback(ctx, strat, apiUrl, jwt, q)
				if len(fb) > 0 {
					specs = fb
					fmt.Printf("using %d specs from provider-locations (fallback) for: %s\n", len(specs), q)
				}
			}
		}
	}
	if len(specs) == 0 {
		specs = []*connect.ProviderSpec{{BestAvailable: true}}
	}
	appVer := fmt.Sprintf("urnet-client %s", Version)
	gen := connect.NewApiMultiClientGeneratorWithDefaults(
		ctx, specs, strat, nil, apiUrl, jwt, fmt.Sprintf("%s/", connectUrl), "", "", appVer, nil,
	)

	// Counters
	var pktsIn, bytesIn, pktsOut, bytesOut uint64

	receive := func(source connect.TransferPath, provideMode protocol.ProvideMode, ipPath *connect.IpPath, packet []byte) {
		if debugOn || isDebugEnabled() {
			logInfo("<- provider len=%d src=%v mode=%v ipPath=%v\n", len(packet), source, provideMode, ipPath)
		}
		_, _ = dev.Write(packet)
		atomic.AddUint64(&pktsIn, 1)
		atomic.AddUint64(&bytesIn, uint64(len(packet)))
	}

	mc := connect.NewRemoteUserNatMultiClientWithDefaults(ctx, gen, receive, protocol.ProvideMode_Network)
	_ = mc

	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := dev.Read(buf)
			if err != nil {
				return
			}
			if n <= 0 {
				continue
			}
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			if debugOn || isDebugEnabled() {
				logInfo("-> provider len=%d\n", len(pkt))
			}
			mc.SendPacket(connect.TransferPath{}, protocol.ProvideMode_Network, pkt, -1)
			atomic.AddUint64(&pktsOut, 1)
			atomic.AddUint64(&bytesOut, uint64(len(pkt)))
		}
	}()

	if statsInt > 0 && isInfoEnabled() {
		go func() {
			t := time.NewTicker(statsInt)
			defer t.Stop()
			for range t.C {
				inP := atomic.LoadUint64(&pktsIn)
				inB := atomic.LoadUint64(&bytesIn)
				outP := atomic.LoadUint64(&pktsOut)
				outB := atomic.LoadUint64(&bytesOut)
				logInfo("[stats] in=%d pkts / %d bytes, out=%d pkts / %d bytes\n", inP, inB, outP, outB)
			}
		}()
	}

	// Optional SOCKS5 proxy bound to the VPN interface
	var stopSocks func() error
	if socksListen != "" {
		if s, err := StartSocks5(ctx, socksListen, tunName, debugOn || isDebugEnabled(), allowDomains, excludeDomains); err != nil {
			logWarn("failed to start socks at %s: %v\n", socksListen, err)
		} else {
			stopSocks = s
			logInfo("SOCKS5 listening at %s (bound to %s)\n", socksListen, tunName)
		}
	}

	if isInfoEnabled() {
		fmt.Println("VPN dataplane running; press Ctrl-C to exit. Note: you must configure routes/DNS to use it.")
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	// Cleanup on exit: stop socks, delete routes, bring link down, and delete address
	if stopSocks != nil {
		_ = stopSocks()
	}
	if defRoute {
		// Remove our TUN default
		_ = run("ip", "route", "del", "default", "dev", tunName)
		// Restore the original default route metric if known
		if origDev != "" {
			var args []string
			if origGw != "" {
				args = []string{"route", "replace", "default", "via", origGw, "dev", origDev}
			} else {
				args = []string{"route", "replace", "default", "dev", origDev}
			}
			if origMetric >= 0 {
				args = append(args, "metric", strconv.Itoa(origMetric))
			}
			_ = run("ip", args...)
		}
		if strings.TrimSpace(excludeRoutes) != "" {
			for _, r := range strings.Split(excludeRoutes, ",") {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				_ = run("ip", "route", "del", r)
			}
		}
	}
	if strings.TrimSpace(extraRoutes) != "" {
		for _, r := range strings.Split(extraRoutes, ",") {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			_ = run("ip", "route", "del", r)
		}
	}
	if !defRoute && strings.TrimSpace(dnsList) != "" {
		for _, d := range strings.Split(dnsList, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			_ = run("ip", "route", "del", d)
		}
	}
	_ = run("ip", "link", "set", tunName, "down")
	_ = run("ip", "addr", "flush", "dev", tunName)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// linuxGetDefaultRoute parses the current default route via `ip -o route show default`.
// Returns gw (may be empty), dev, metric (or -1 if not present).
func linuxGetDefaultRoute() (string, string, int, error) {
	out, err := runCapture("ip", "-o", "route", "show", "default")
	if err != nil {
		return "", "", -1, err
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return "", "", -1, errors.New("no default route")
	}
	// Example: "default via 172.17.0.254 dev eth0 proto dhcp src 172.17.0.11 metric 100"
	// Or:     "default dev eth0 proto kernel  scope link src 10.0.2.15"
	toks := strings.Fields(line)
	var gw, dev string
	met := -1
	for i := 0; i < len(toks); i++ {
		switch toks[i] {
		case "via":
			if i+1 < len(toks) {
				gw = toks[i+1]
				i++
			}
		case "dev":
			if i+1 < len(toks) {
				dev = toks[i+1]
				i++
			}
		case "metric":
			if i+1 < len(toks) {
				if v, e := strconv.Atoi(toks[i+1]); e == nil {
					met = v
				}
				i++
			}
		}
	}
	if dev == "" {
		return "", "", -1, errors.New("default route parse failed: no dev")
	}
	return gw, dev, met, nil
}

// runCapture runs a command and returns stdout as string.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(b), nil
}
