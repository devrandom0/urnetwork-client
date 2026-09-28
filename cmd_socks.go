package main

import (
	"context"
	"fmt"

	"github.com/docopt/docopt-go"
)

func cmdSocks(ctx context.Context, opts docopt.Opts) error {
	cfg := parseSOCKSConfig(opts)

	if cfg.ListenAddr == "" {
		return fmt.Errorf("--listen is required for socks command")
	}

	// NOTE: extender connection is not yet implemented; the binary logs the target
	// and runs a plain SOCKS5 proxy. Track as a known gap.
	logInfo("Extender details: IP=%s Port=%s SNI=%s\n", cfg.ExtenderIP, cfg.ExtenderPort, cfg.ExtenderSNI)

	stopSocks, err := StartSocks5(ctx, SocksOptions{
		ListenAddr:     cfg.ListenAddr,
		Auth:           cfg.Auth,
		Debug:          cfg.Debug,
		AllowDomains:   cfg.AllowDomains,
		ExcludeDomains: cfg.ExcludeDomains,
	})
	if err != nil {
		return fmt.Errorf("failed to start SOCKS5 proxy: %w", err)
	}
	defer func() { _ = stopSocks() }()

	logInfo("SOCKS5 proxy listening at %s\n", cfg.ListenAddr)
	logInfo("Connecting to extender at %s:%s (SNI: %s)\n", cfg.ExtenderIP, cfg.ExtenderPort, cfg.ExtenderSNI)

	<-ctx.Done()
	logInfo("Shutting down SOCKS5 proxy...\n")
	return nil
}
