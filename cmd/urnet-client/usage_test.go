package main

import (
	"strings"
	"testing"

	"github.com/docopt/docopt-go"
)

func TestUsage_DNSBootstrapDefaultsToNone(t *testing.T) {
	opts, err := docopt.ParseArgs(usageText(), []string{"vpn", "--tun=none"}, Version)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, _ := opts.String("--dns_bootstrap"); got != "none" {
		t.Fatalf("--dns_bootstrap default = %q, want none", got)
	}
}

func TestUsage_DescribesDNSServiceAsIgnored(t *testing.T) {
	for _, line := range strings.Split(usageText(), "\n") {
		if strings.Contains(line, "--dns_service=<name>  ") {
			if !strings.Contains(line, "Ignored") {
				t.Fatalf("help line %q; --dns_service no longer does anything and must say so", strings.TrimSpace(line))
			}
			return
		}
	}
	t.Fatal("no --dns_service option line in the usage text")
}
