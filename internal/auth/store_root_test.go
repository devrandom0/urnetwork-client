//go:build safefiletest

package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/safefile"
)

func TestLoad_RefusesSymlinkedJWTFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("URNETWORK_HOME", home)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, "jwt")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(safefile.SetEUIDForTesting(0))
	if _, err := Load(""); err == nil {
		t.Fatal("Load followed a symlinked jwt file as root")
	}
}

func TestLoad_FollowsSymlinkedJWTFileWhenNotRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("URNETWORK_HOME", home)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("tok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, "jwt")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(safefile.SetEUIDForTesting(4242))
	if got, err := Load(""); err != nil || got != "tok" {
		t.Fatalf("got %q, %v", got, err)
	}
}
