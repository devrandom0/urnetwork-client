package main

import (
	"fmt"
	"os"
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

// mustString returns the string value for key, printing an error and exiting if blank.
func mustString(opts docopt.Opts, key string) string {
	v, _ := opts.String(key)
	if strings.TrimSpace(v) == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", key)
		os.Exit(2)
	}
	return v
}

// mustBool returns the bool value of key from opts, ignoring errors.
func mustBool(opts docopt.Opts, key string) bool {
	b, _ := opts.Bool(key)
	return b
}

// fatal prints an error to stderr and exits with code 1.
func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

// idsToStrings converts a slice of connect.Id to a slice of their string representations.
func idsToStrings(ids []connect.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
