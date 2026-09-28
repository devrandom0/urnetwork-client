//go:build linux

package netcfg

// The last entry is NixOS's system profile.
var systemToolDirs = []string{"/sbin", "/usr/sbin", "/bin", "/usr/bin", "/run/current-system/sw/bin"}
