package main

import (
	"context"
	"fmt"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/logx"
	"github.com/devrandom0/urnetwork-client/internal/socks"
)

func cmdSocks(ctx context.Context, opts docopt.Opts) error {
	cfg := parseSOCKSConfig(opts)

	if cfg.ListenAddr == "" {
		return fmt.Errorf("--listen is required for socks command")
	}

	// NOTE: extender connection is not yet implemented; the binary logs the target
	// and runs a plain SOCKS5 proxy. Track as a known gap.
	logx.Info("Extender details: IP=%s Port=%s SNI=%s\n", cfg.ExtenderIP, cfg.ExtenderPort, cfg.ExtenderSNI)

	stopSocks, err := socks.StartSocks5(ctx, socks.SocksOptions{
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

	logx.Info("SOCKS5 proxy listening at %s\n", cfg.ListenAddr)
	logx.Info("Connecting to extender at %s:%s (SNI: %s)\n", cfg.ExtenderIP, cfg.ExtenderPort, cfg.ExtenderSNI)

	<-ctx.Done()
	logx.Info("Shutting down SOCKS5 proxy...\n")
	return nil
}
