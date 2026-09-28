// Package netcfg manages OS-level routes, TUN interfaces and gateway/DNS discovery for the VPN.
package netcfg

import (
	"os"
	"os/exec"
)

// commandRunner is the single choke point for OS commands so route and TUN logic can be
// tested without root.
type commandRunner interface {
	Run(name string, args ...string) error
	Capture(name string, args ...string) (string, error)
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

var cmdRunner commandRunner = execRunner{}

// runCapture executes a command and returns its combined stdout+stderr output and any error.
func runCapture(name string, args ...string) (string, error) {
	return cmdRunner.Capture(name, args...)
}
