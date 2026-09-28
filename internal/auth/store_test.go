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
	if _, err := Load(""); err == nil {
		t.Fatal("Load followed a symlinked jwt file")
	}
}

func TestLoad_RefusesJWTFileOver64KiB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("URNETWORK_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "jwt"), make([]byte, 64<<10+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(""); err == nil {
		t.Fatal("Load accepted a jwt file above 64 KiB")
	}
}

func TestLoad_ReadsSavedToken(t *testing.T) {
	t.Setenv("URNETWORK_HOME", t.TempDir())
	if err := Save("tok"); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(""); err != nil || got != "tok" {
		t.Fatalf("got %q, %v", got, err)
	}
}
