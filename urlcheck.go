package main

import (
	"fmt"
	"net"
	neturl "net/url"
	"strings"

	"github.com/docopt/docopt-go"
)

// validateEndpointURL allows the insecure devScheme only for loopback hosts, so a typo
// cannot send JWTs in cleartext across the network.
func validateEndpointURL(name, raw, secureScheme, devScheme string) error {
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s: invalid URL %q (want %s://host)", name, raw, secureScheme)
	}
	switch strings.ToLower(u.Scheme) {
	case secureScheme:
		return nil
	case devScheme:
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("%s: %s:// is only allowed for localhost; use %s://", name, devScheme, secureScheme)
	default:
		return fmt.Errorf("%s: unsupported scheme %q (want %s://)", name, u.Scheme, secureScheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateEndpointFlags(opts docopt.Opts) error {
	if v := getStringOr(opts, "--api_url", ""); v != "" {
		if err := validateEndpointURL("--api_url", v, "https", "http"); err != nil {
			return err
		}
	}
	if v := getStringOr(opts, "--connect_url", ""); v != "" {
		if err := validateEndpointURL("--connect_url", v, "wss", "ws"); err != nil {
			return err
		}
	}
	return nil
}
