package main

import (
	"testing"

	"github.com/docopt/docopt-go"
)

func TestValidateEndpointURL(t *testing.T) {
	cases := []struct {
		name, raw, secure, dev string
		ok                     bool
	}{
		{"https api", "https://api.bringyour.com", "https", "http", true},
		{"http remote api", "http://api.bringyour.com", "https", "http", false},
		{"http localhost", "http://localhost:8080", "https", "http", true},
		{"http 127.0.0.1", "http://127.0.0.1:8080", "https", "http", true},
		{"http ::1", "http://[::1]:8080", "https", "http", true},
		{"wss connect", "wss://connect.bringyour.com", "wss", "ws", true},
		{"ws remote connect", "ws://connect.bringyour.com", "wss", "ws", false},
		{"ws localhost", "ws://localhost:9000", "wss", "ws", true},
		{"wrong scheme", "ftp://api.bringyour.com", "https", "http", false},
		{"no scheme", "api.bringyour.com", "https", "http", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEndpointURL("--x", tc.raw, tc.secure, tc.dev)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestValidateEndpointFlags(t *testing.T) {
	if err := validateEndpointFlags(docopt.Opts{"--api_url": DefaultAPIURL, "--connect_url": DefaultConnectURL}); err != nil {
		t.Fatalf("defaults must pass: %v", err)
	}
	if err := validateEndpointFlags(docopt.Opts{"--api_url": "http://api.example.com"}); err == nil {
		t.Fatal("cleartext remote api_url must fail")
	}
	if err := validateEndpointFlags(docopt.Opts{}); err != nil {
		t.Fatalf("commands without URL flags must pass: %v", err)
	}
}
