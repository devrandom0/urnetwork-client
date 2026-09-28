package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSave_WritesTrimmedTokenWith0600(t *testing.T) {
	home := t.TempDir()
	t.Setenv("URNETWORK_HOME", home)
	if err := Save("  tok  "); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "jwt")
	b, err := os.ReadFile(path)
	fi, _ := os.Stat(path)
	if err != nil || string(b) != "tok\n" || fi.Mode().Perm() != 0o600 {
		t.Fatalf("content=%q mode=%v err=%v", b, fi.Mode(), err)
	}
}

func TestSave_RefusesSymlinkedJWTFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("URNETWORK_HOME", home)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, "jwt")); err != nil {
		t.Fatal(err)
	}
	if err := Save("tok"); err == nil {
		t.Fatal("Save wrote through a symlinked jwt file")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("symlink target overwritten: %q", b)
	}
}
