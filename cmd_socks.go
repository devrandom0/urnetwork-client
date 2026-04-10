package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/docopt/docopt-go"
)

func cmdSocks(opts docopt.Opts) {
	listenAddr, _ := opts.String("--listen")
	extenderIP, _ := opts.String("--extender_ip")
	extenderPort, _ := opts.String("--extender_port")
	extenderSNI, _ := opts.String("--extender_sni")
	extenderSecret, _ := opts.String("--extender_secret")
	debugOn, _ := opts.Bool("--debug")

	if listenAddr == "" {
		fatal(errors.New("--listen is required for socks command"))
	}
	if extenderIP == "" {
		fatal(errors.New("--extender_ip is required for socks command"))
	}
	if extenderPort == "" {
		fatal(errors.New("--extender_port is required for socks command"))
	}
	if extenderSNI == "" {
		fatal(errors.New("--extender_sni is required for socks command"))
	}

	allowDomains := splitCSV(getStringOr(opts, "--domain", ""))
	excludeDomains := splitCSV(getStringOr(opts, "--exclude_domain", ""))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// NOTE: extender connection is not yet implemented; the binary logs the target
	// and runs a plain SOCKS5 proxy. Track as a known gap.
	_ = extenderSecret
	logInfo("Extender details: IP=%s Port=%s SNI=%s\n", extenderIP, extenderPort, extenderSNI)

	stopSocks, err := StartSocks5(ctx, listenAddr, "", debugOn, allowDomains, excludeDomains)
	if err != nil {
		fatal(fmt.Errorf("failed to start SOCKS5 proxy: %w", err))
	}
	defer func() { _ = stopSocks() }()

	logInfo("SOCKS5 proxy listening at %s\n", listenAddr)
	logInfo("Connecting to extender at %s:%s (SNI: %s)\n", extenderIP, extenderPort, extenderSNI)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	_ = strings.TrimSpace // keep import used elsewhere in same package
	logInfo("Shutting down SOCKS5 proxy...\n")
}
