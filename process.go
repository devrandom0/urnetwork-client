package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// spawnBackground detaches a child copy of this process (dropping --background) and returns its PID.
func spawnBackground(argv []string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	args := make([]string, 0, len(argv)-1)
	for i, a := range argv {
		if i == 0 {
			continue // skip program name
		}
		if a == "--background" || strings.HasPrefix(a, "--background=") {
			continue
		}
		args = append(args, a)
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// setupLogFile redirects os.Stdout and os.Stderr to the given file path, appending if it exists.
// The file is created with 0o600 permissions to protect potentially sensitive log content.
func setupLogFile(path string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	os.Stdout = f
	os.Stderr = f
	return nil
}
