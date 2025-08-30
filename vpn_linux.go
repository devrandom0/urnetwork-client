//go:build linux

package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"
    "sync/atomic"
    "time"
    "os/exec"
    "strings"

    "github.com/docopt/docopt-go"
    "github.com/songgao/water"

    "github.com/urnetwork/connect"
    "github.com/urnetwork/connect/protocol"
)

func cmdVpn(opts docopt.Opts) {
    apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
    connectUrl := getStringOr(opts, "--connect_url", DefaultConnectUrl)
    tunName := getStringOr(opts, "--tun", "urnet0")
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
    if err != nil { fatal(err) }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

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
    if defRoute {
        _ = run("ip", "route", "add", "default", "dev", tunName)
    if strings.TrimSpace(excludeRoutes) != "" {
            for _, r := range strings.Split(excludeRoutes, ",") {
                r = strings.TrimSpace(r)
                if r == "" { continue }
                // Exclude by adding unreachable route so traffic won't go via default TUN
                _ = run("ip", "route", "add", r, "unreachable")
            }
        }
    }
    if strings.TrimSpace(extraRoutes) != "" {
        for _, r := range strings.Split(extraRoutes, ",") {
            r = strings.TrimSpace(r)
            if r == "" { continue }
            _ = run("ip", "route", "add", r, "dev", tunName)
        }
    }
    if !defRoute && strings.TrimSpace(dnsList) != "" {
        for _, d := range strings.Split(dnsList, ",") {
            d = strings.TrimSpace(d)
            if d == "" { continue }
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
            if err != nil { return }
            if n <= 0 { continue }
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

    if isInfoEnabled() { fmt.Println("VPN dataplane running; press Ctrl-C to exit. Note: you must configure routes/DNS to use it.") }
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
    <-sig

    // Cleanup on exit: stop socks, delete routes, bring link down, and delete address
    if stopSocks != nil { _ = stopSocks() }
    if defRoute {
        _ = run("ip", "route", "del", "default")
        if strings.TrimSpace(excludeRoutes) != "" {
            for _, r := range strings.Split(excludeRoutes, ",") {
                r = strings.TrimSpace(r)
                if r == "" { continue }
                _ = run("ip", "route", "del", r)
            }
        }
    }
    if strings.TrimSpace(extraRoutes) != "" {
        for _, r := range strings.Split(extraRoutes, ",") {
            r = strings.TrimSpace(r)
            if r == "" { continue }
            _ = run("ip", "route", "del", r)
        }
    }
    if !defRoute && strings.TrimSpace(dnsList) != "" {
        for _, d := range strings.Split(dnsList, ",") {
            d = strings.TrimSpace(d)
            if d == "" { continue }
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
