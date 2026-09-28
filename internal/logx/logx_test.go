package logx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetLogLevel(t *testing.T) {
	SetLogLevel("info", false)
	if !IsInfoEnabled() || IsDebugEnabled() {
		t.Fatalf("info should enable info but not debug")
	}
	SetLogLevel("debug", false)
	if !IsDebugEnabled() {
		t.Fatalf("debug should enable debug")
	}
	SetLogLevel("warn", false)
	if !IsWarnEnabled() || IsInfoEnabled() {
		t.Fatalf("warn should disable info")
	}
	SetLogLevel("error", false)
	if !IsErrorEnabled() || IsWarnEnabled() {
		t.Fatalf("error should disable warn")
	}
	SetLogLevel("quiet", false)
	if LogLevel(currentLogLevel.Load()) != LevelQuiet {
		t.Fatalf("quiet should set LevelQuiet")
	}
	// --debug implies debug when no explicit level
	SetLogLevel("", true)
	if !IsDebugEnabled() {
		t.Fatalf("debug flag should imply debug level when no level is set")
	}
}

func TestSetupLogFile_RefusesSymlink(t *testing.T) {
	origOut, origErr := os.Stdout, os.Stderr
	t.Cleanup(func() { os.Stdout, os.Stderr = origOut, origErr })
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "urnet.log")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := SetupLogFile(link); err == nil {
		t.Fatal("SetupLogFile opened a symlinked log path")
	}
	if os.Stdout != origOut {
		t.Fatal("stdout redirected despite the error")
	}
}

func TestSetupLogFile_CreatesLogDir0700(t *testing.T) {
	origOut, origErr := os.Stdout, os.Stderr
	t.Cleanup(func() { os.Stdout, os.Stderr = origOut, origErr })
	dir := filepath.Join(t.TempDir(), "logs")
	if err := SetupLogFile(filepath.Join(dir, "urnet.log")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Stdout.Close() })
	fi, err := os.Stat(dir)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("log dir mode=%v err=%v, want 0700", fi.Mode(), err)
	}
}
