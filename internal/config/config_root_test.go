//go:build safefiletest

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/safefile"
)

func TestLoadConfigFile_RefusesSymlink(t *testing.T) {
	real := filepath.Join(t.TempDir(), "real.yaml")
	if err := os.WriteFile(real, []byte("debug: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "urnet.yaml")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(safefile.SetEUIDForTesting(0))
	if _, err := loadConfigFile(link); err == nil {
		t.Fatal("loadConfigFile followed a symlink as root")
	}
}

func TestLoadConfigFile_FollowsSymlinkWhenNotRoot(t *testing.T) {
	real := filepath.Join(t.TempDir(), "real.yaml")
	if err := os.WriteFile(real, []byte("debug: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "urnet.yaml")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(safefile.SetEUIDForTesting(4242))
	cf, err := loadConfigFile(link)
	if err != nil || !cf.Debug {
		t.Fatalf("got %+v, %v", cf, err)
	}
}
