// Package netcfg manages OS-level routes, TUN interfaces and gateway/DNS discovery for the VPN.
package netcfg

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// commandRunner is the single choke point for OS commands so route and TUN logic can be
// tested without root.
type commandRunner interface {
	Run(name string, args ...string) error
	Capture(name string, args ...string) (string, error)
	CaptureStdout(name string, args ...string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) error {
	path, err := tools.resolve(name)
	if err != nil {
		return err
	}
	cmd := exec.Command(path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (execRunner) Capture(name string, args ...string) (string, error) {
	path, err := tools.resolve(name)
	if err != nil {
		return "", err
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	return string(out), err
}

func (execRunner) CaptureStdout(name string, args ...string) (string, error) {
	path, err := tools.resolve(name)
	if err != nil {
		return "", err
	}
	out, err := exec.Command(path, args...).Output()
	// Output() doesn't parse stdout, but on failure its ExitError carries no stderr text at
	// all by default; fold it into the error so a route/ip/ifconfig failure stays diagnosable.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			err = fmt.Errorf("%w: %s", err, stderr)
		}
	}
	return string(out), err
}

var cmdRunner commandRunner = execRunner{}

// runCapture executes a command and returns its combined stdout+stderr output and any error.
func runCapture(name string, args ...string) (string, error) {
	return cmdRunner.Capture(name, args...)
}

// runCaptureStdout executes a command and returns its stdout only, so stray text a tool
// writes to stderr (warnings, locale-dependent noise) cannot corrupt parsed output.
func runCaptureStdout(name string, args ...string) (string, error) {
	return cmdRunner.CaptureStdout(name, args...)
}
