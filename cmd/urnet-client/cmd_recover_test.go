package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/netcfg"
)

type fakeRecover struct {
	calls   []netcfg.RecoverOptions
	actions []string
	err     error
}

func (f *fakeRecover) run(o netcfg.RecoverOptions) ([]string, error) {
	f.calls = append(f.calls, o)
	return f.actions, f.err
}

func recoverTestDeps(euid int, f *fakeRecover, out *bytes.Buffer) recoverDeps {
	return recoverDeps{geteuid: func() int { return euid }, recover: f.run, out: out}
}

func parseRecover(t *testing.T, args ...string) docopt.Opts {
	t.Helper()
	opts, err := docopt.ParseArgs(usageText(), append([]string{"recover"}, args...), Version)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return opts
}

func TestRecover_RefusesWithoutRoot(t *testing.T) {
	f := &fakeRecover{}
	err := cmdRecover(parseRecover(t), recoverTestDeps(501, f, &bytes.Buffer{}))

	if err == nil || !strings.Contains(err.Error(), "sudo urnet-client recover") {
		t.Fatalf("cmdRecover = %v; want a root error naming the sudo command", err)
	}
	if len(f.calls) != 0 {
		t.Fatal("recovery ran without root")
	}
}

func TestRecover_PrintsEachAction(t *testing.T) {
	f := &fakeRecover{actions: []string{"removed the DNS entry X", "removed the IPv6 reject route ::/1"}}
	var out bytes.Buffer

	if err := cmdRecover(parseRecover(t), recoverTestDeps(0, f, &out)); err != nil {
		t.Fatalf("cmdRecover: %v", err)
	}
	if out.String() != "removed the DNS entry X\nremoved the IPv6 reject route ::/1\n" {
		t.Fatalf("output = %q", out.String())
	}
	if len(f.calls) != 1 || f.calls[0] != (netcfg.RecoverOptions{}) {
		t.Fatalf("calls = %+v; want one run without --force", f.calls)
	}
}

func TestRecover_SaysNothingToRecover(t *testing.T) {
	var out bytes.Buffer
	if err := cmdRecover(parseRecover(t), recoverTestDeps(0, &fakeRecover{}, &out)); err != nil {
		t.Fatalf("cmdRecover: %v", err)
	}
	if out.String() != "nothing to recover\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRecover_ForceFlagReachesRecovery(t *testing.T) {
	f := &fakeRecover{}
	if err := cmdRecover(parseRecover(t, "--force"), recoverTestDeps(0, f, &bytes.Buffer{})); err != nil {
		t.Fatalf("cmdRecover: %v", err)
	}
	if len(f.calls) != 1 || !f.calls[0].Force {
		t.Fatalf("calls = %+v; want Force set", f.calls)
	}
}

func TestRecover_LiveOwnerErrorSuggestsForce(t *testing.T) {
	f := &fakeRecover{err: &netcfg.LiveOwnerError{PID: 4242}}

	err := cmdRecover(parseRecover(t), recoverTestDeps(0, f, &bytes.Buffer{}))

	if err == nil || !strings.Contains(err.Error(), "pid 4242") || !strings.Contains(err.Error(), "sudo urnet-client recover --force") {
		t.Fatalf("cmdRecover = %v; want the owner pid and the --force command", err)
	}
}

func TestRecover_PrintsActionsBeforeReportingAFailure(t *testing.T) {
	f := &fakeRecover{actions: []string{"removed the DNS entry X"}, err: errors.New("delete IPv6 reject route ::/1: exit status 1")}
	var out bytes.Buffer

	err := cmdRecover(parseRecover(t), recoverTestDeps(0, f, &out))

	if err == nil || !strings.Contains(out.String(), "removed the DNS entry X") {
		t.Fatalf("cmdRecover = %v, output %q; want the done steps printed and the failure returned", err, out.String())
	}
}
