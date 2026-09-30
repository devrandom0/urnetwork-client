//go:build windows

package main

import "testing"

// This only builds and vets on windows (there's no windows runner in this repo's gate), but it
// pins the behavior: --background must fail clearly instead of starting a non-detached child.
func TestSpawnBackground_RefusesOnWindows(t *testing.T) {
	if _, err := spawnBackground([]string{"urnet-client"}, nil); err == nil {
		t.Fatal("spawnBackground must refuse --background on windows")
	}
}

func TestBackgroundSupported_FalseOnWindows(t *testing.T) {
	if err := backgroundSupported(); err == nil {
		t.Fatal("backgroundSupported must report an error on windows")
	}
}
