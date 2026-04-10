package main

import (
	neturl "net/url"
	"os/exec"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/urnetwork/connect"
)

// splitCSV splits a comma-separated list, trimming whitespace and removing empties.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// runCapture executes a command and returns its combined stdout+stderr output and any error.
func runCapture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// getStringOr returns the string value for key from opts, or def if missing/empty.
func getStringOr(opts docopt.Opts, key, def string) string {
	if v, err := opts.String(key); err == nil && v != "" {
		return v
	}
	return def
}

// getIntOr returns the int value for key from opts, or def if missing.
func getIntOr(opts docopt.Opts, key string, def int) int {
	if v, err := opts.Int(key); err == nil {
		return v
	}
	return def
}

// mustBool returns the bool value of key from opts, ignoring errors.
func mustBool(opts docopt.Opts, key string) bool {
	b, _ := opts.Bool(key)
	return b
}

// idsToStrings converts a slice of connect.Id to a slice of their string representations.
func idsToStrings(ids []connect.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

// extractHost parses rawURL as a URL or bare hostname and returns just the hostname.
// Port numbers and path components are stripped. Returns "" when rawURL is empty.
func extractHost(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := neturl.Parse(rawURL)
	if err == nil && u.Host != "" {
		host := u.Host
		if i := strings.Index(host, ":"); i >= 0 {
			host = host[:i]
		}
		return host
	}
	// Treat as bare host or IP; strip any trailing path.
	host := rawURL
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	return host
}
