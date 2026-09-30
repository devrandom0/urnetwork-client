// Package netcfg manages OS-level routes, TUN interfaces and gateway/DNS discovery for the VPN.
package netcfg

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// commandRunner is the single choke point for OS commands so route and TUN logic can be
// tested without root.
type commandRunner interface {
	Run(name string, args ...string) error
	Capture(name string, args ...string) (string, error)
	// RunInput feeds stdin to the command and returns its combined output.
	RunInput(stdin, name string, args ...string) (string, error)
	CaptureStdout(name string, args ...string) (string, error)
}

type execRunner struct{}

// commandTimeout bounds every route, ip, ifconfig, scutil and resolvectl call, so one hung
// tool cannot stall startup or the cleanup that restores DNS.
var commandTimeout = 5 * time.Second

// command also bounds the wait for pipes a tool's own children might keep open.
func command(name string, args ...string) (*exec.Cmd, context.CancelFunc, error) {
	path, err := tools.resolve(name)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.WaitDelay = time.Second
	return cmd, cancel, nil
}

// Run captures the tool's output instead of sharing the process's stdout and stderr: a
// terminal or pipe that stops reading would block the tool, and with it the caller.
func (execRunner) Run(name string, args ...string) error {
	cmd, cancel, err := command(name, args...)
	if err != nil {
		return err
	}
	defer cancel()
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text != "" {
		return fmt.Errorf("%w: %s", err, text)
	}
	if text != "" {
		logx.Debug("%s: %s\n", name, text)
	}
	return err
}

func (execRunner) Capture(name string, args ...string) (string, error) {
	cmd, cancel, err := command(name, args...)
	if err != nil {
		return "", err
	}
	defer cancel()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (execRunner) RunInput(stdin, name string, args ...string) (string, error) {
	cmd, cancel, err := command(name, args...)
	if err != nil {
		return "", err
	}
	defer cancel()
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (execRunner) CaptureStdout(name string, args ...string) (string, error) {
	cmd, cancel, err := command(name, args...)
	if err != nil {
		return "", err
	}
	defer cancel()
	out, err := cmd.Output()
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
