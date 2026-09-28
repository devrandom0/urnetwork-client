package netcfg

import (
	neturl "net/url"
	"strings"
)

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
