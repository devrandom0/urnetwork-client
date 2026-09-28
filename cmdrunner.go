package main

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
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (execRunner) Capture(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

var cmdRunner commandRunner = execRunner{}
