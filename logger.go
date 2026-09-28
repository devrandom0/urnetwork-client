package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type LogLevel int32

const (
	LevelQuiet LogLevel = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

// currentLogLevel is accessed atomically so reads from the dataplane goroutine
// are race-free when setLogLevel is called from the main goroutine.
var currentLogLevel atomic.Int32

func init() {
	currentLogLevel.Store(int32(LevelInfo))
}

func setLogLevel(level string, debugFlag bool) {
	lvl := strings.ToLower(strings.TrimSpace(level))
	var l LogLevel
	switch lvl {
	case "quiet", "silent":
		l = LevelQuiet
	case "error", "err":
		l = LevelError
	case "warn", "warning":
		l = LevelWarn
	case "debug":
		l = LevelDebug
	case "info", "":
		l = LevelInfo
		if level == "" && debugFlag {
			l = LevelDebug
		}
	default:
		l = LevelInfo
	}
	currentLogLevel.Store(int32(l))
}

func isDebugEnabled() bool { return LogLevel(currentLogLevel.Load()) >= LevelDebug }
func isInfoEnabled() bool  { return LogLevel(currentLogLevel.Load()) >= LevelInfo }
func isWarnEnabled() bool  { return LogLevel(currentLogLevel.Load()) >= LevelWarn }
func isErrorEnabled() bool { return LogLevel(currentLogLevel.Load()) >= LevelError }

// logf writes a structured log line to w with the current UTC timestamp and a level tag.
// Format: "2006-01-02T15:04:05Z [LEVEL] message"
func logf(w *os.File, tag, format string, args ...any) {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(w, "%s [%s] %s", ts, tag, msg)
}

func logInfo(format string, args ...any) {
	if isInfoEnabled() {
		logf(os.Stdout, "INFO", format, args...)
	}
}

func logWarn(format string, args ...any) {
	if isWarnEnabled() {
		logf(os.Stderr, "WARN", format, args...)
	}
}

func logError(format string, args ...any) {
	if isErrorEnabled() {
		logf(os.Stderr, "ERROR", format, args...)
	}
}

func logDebug(format string, args ...any) {
	if isDebugEnabled() {
		logf(os.Stdout, "DEBUG", format, args...)
	}
}
