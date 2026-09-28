package logx

import "testing"

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
