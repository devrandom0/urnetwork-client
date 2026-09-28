//go:build darwin

package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type countingRemover struct{ n atomic.Int32 }

func (c *countingRemover) RemoveDNSBypass() { c.n.Add(1) }

func TestRemoveDNSBypassWhenWarm_RemovesOnceTrafficFlows(t *testing.T) {
	var in, out uint64 = 1, 1
	r := &countingRemover{}
	removeDNSBypassWhenWarm(context.Background(), r, &in, &out, time.Minute, time.Millisecond)
	if r.n.Load() != 1 {
		t.Fatalf("removals = %d, want 1", r.n.Load())
	}
}

func TestRemoveDNSBypassWhenWarm_RemovesAtDeadline(t *testing.T) {
	var in, out uint64
	r := &countingRemover{}
	removeDNSBypassWhenWarm(context.Background(), r, &in, &out, 20*time.Millisecond, 5*time.Millisecond)
	if r.n.Load() != 1 {
		t.Fatalf("removals = %d, want 1", r.n.Load())
	}
}

func TestRemoveDNSBypassWhenWarm_StopsOnCancel(t *testing.T) {
	var in, out uint64
	r := &countingRemover{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	removeDNSBypassWhenWarm(ctx, r, &in, &out, time.Minute, time.Minute)
	if r.n.Load() != 0 {
		t.Fatal("must not touch routes after the session context is done; Cleanup owns them then")
	}
}
