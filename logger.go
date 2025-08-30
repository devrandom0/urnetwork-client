package main

import (
	"fmt"
	"os"
	"strings"
)

type LogLevel int

const (
	LevelQuiet LogLevel = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

var currentLogLevel = LevelInfo

func setLogLevel(level string, debugFlag bool) {
	lvl := strings.ToLower(strings.TrimSpace(level))
	switch lvl {
	case "quiet", "silent":
		currentLogLevel = LevelQuiet
	case "error", "err":
		currentLogLevel = LevelError
	case "warn", "warning":
		currentLogLevel = LevelWarn
	case "debug":
		currentLogLevel = LevelDebug
	case "info", "":
		currentLogLevel = LevelInfo
	default:
		currentLogLevel = LevelInfo
	}
	if level == "" && debugFlag {
		currentLogLevel = LevelDebug
	}
}

func isDebugEnabled() bool { return currentLogLevel >= LevelDebug }
func isInfoEnabled() bool  { return currentLogLevel >= LevelInfo }
func isWarnEnabled() bool  { return currentLogLevel >= LevelWarn }
func isErrorEnabled() bool { return currentLogLevel >= LevelError }

func logDebug(format string, args ...any) {
	if isDebugEnabled() {
		fmt.Printf(format, args...)
	}
}

func logInfo(format string, args ...any) {
	if isInfoEnabled() {
		fmt.Printf(format, args...)
	}
}

func logWarn(format string, args ...any) {
	if isWarnEnabled() {
		fmt.Fprintf(os.Stderr, "warn: "+format, args...)
	}
}

func logError(format string, args ...any) {
	if isErrorEnabled() {
		fmt.Fprintf(os.Stderr, "error: "+format, args...)
	}
}
