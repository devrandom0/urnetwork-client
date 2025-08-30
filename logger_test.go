package main

import "testing"

func TestSetLogLevel(t *testing.T) {
	setLogLevel("info", false)
	if !isInfoEnabled() || isDebugEnabled() {
		t.Fatalf("info should enable info but not debug")
	}
	setLogLevel("debug", false)
	if !isDebugEnabled() {
		t.Fatalf("debug should enable debug")
	}
	setLogLevel("warn", false)
	if !isWarnEnabled() || isInfoEnabled() {
		t.Fatalf("warn should disable info")
	}
	setLogLevel("error", false)
	if !isErrorEnabled() || isWarnEnabled() {
		t.Fatalf("error should disable warn")
	}
	setLogLevel("quiet", false)
	if currentLogLevel != LevelQuiet {
		t.Fatalf("quiet should set LevelQuiet")
	}
	// --debug implies debug when no explicit level
	setLogLevel("", true)
	if !isDebugEnabled() {
		t.Fatalf("debug flag should imply debug level when no level is set")
	}
}
