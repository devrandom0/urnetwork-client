package config

import (
	"strings"

	"github.com/docopt/docopt-go"
)

// SplitCSV splits a comma-separated list, trimming whitespace and removing empties.
func SplitCSV(s string) []string {
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

// StringOr returns the string value for key from opts, or def if missing/empty.
func StringOr(opts docopt.Opts, key, def string) string {
	if v, err := opts.String(key); err == nil && v != "" {
		return v
	}
	return def
}

// IntOr returns the int value for key from opts, or def if missing.
func IntOr(opts docopt.Opts, key string, def int) int {
	if v, err := opts.Int(key); err == nil {
		return v
	}
	return def
}

// MustBool returns the bool value of key from opts, ignoring errors.
func MustBool(opts docopt.Opts, key string) bool {
	b, _ := opts.Bool(key)
	return b
}
