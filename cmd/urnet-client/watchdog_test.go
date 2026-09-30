package main

import (
	"errors"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/netcfg"
)

func TestWatchdogRecovery_ActsOnlyForItsOwner(t *testing.T) {
	var got netcfg.RecoverOptions
	recoverFn := watchdogRecovery(func(o netcfg.RecoverOptions) ([]string, error) {
		got = o
		return []string{"removed the DNS entry X"}, nil
	})

	if err := recoverFn(4242); err != nil {
		t.Fatalf("recover = %v", err)
	}
	if got != (netcfg.RecoverOptions{Owner: 4242}) {
		t.Fatalf("options = %+v; want Owner set and no Force", got)
	}
}

func TestWatchdogRecovery_NotOwnerIsNotAnError(t *testing.T) {
	recoverFn := watchdogRecovery(func(netcfg.RecoverOptions) ([]string, error) { return nil, netcfg.ErrNotOwner })

	if err := recoverFn(4242); err != nil {
		t.Fatalf("recover = %v; DNS already restored or owned by a newer run means nothing to do", err)
	}
}

func TestWatchdogRecovery_ReportsFailure(t *testing.T) {
	want := errors.New("scutil: Permission denied")
	recoverFn := watchdogRecovery(func(netcfg.RecoverOptions) ([]string, error) { return nil, want })

	if err := recoverFn(4242); !errors.Is(err, want) {
		t.Fatalf("recover = %v", err)
	}
}
