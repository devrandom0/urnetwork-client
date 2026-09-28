// Package logx provides leveled, timestamped logging for the CLI.
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/safefile"
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
// are race-free when SetLogLevel is called from the main goroutine.
var currentLogLevel atomic.Int32

func init() {
	currentLogLevel.Store(int32(LevelInfo))
}

func SetLogLevel(level string, debugFlag bool) {
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

func IsDebugEnabled() bool { return LogLevel(currentLogLevel.Load()) >= LevelDebug }
func IsInfoEnabled() bool  { return LogLevel(currentLogLevel.Load()) >= LevelInfo }
func IsWarnEnabled() bool  { return LogLevel(currentLogLevel.Load()) >= LevelWarn }
func IsErrorEnabled() bool { return LogLevel(currentLogLevel.Load()) >= LevelError }

// logf writes a structured log line to w with the current UTC timestamp and a level tag.
// Format: "2006-01-02T15:04:05Z [LEVEL] message"
func logf(w *os.File, tag, format string, args ...any) {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(w, "%s [%s] %s", ts, tag, msg)
}

func Info(format string, args ...any) {
	if IsInfoEnabled() {
		logf(os.Stdout, "INFO", format, args...)
	}
}

func Warn(format string, args ...any) {
	if IsWarnEnabled() {
		logf(os.Stderr, "WARN", format, args...)
	}
}

func Error(format string, args ...any) {
	if IsErrorEnabled() {
		logf(os.Stderr, "ERROR", format, args...)
	}
}

func Debug(format string, args ...any) {
	if IsDebugEnabled() {
		logf(os.Stdout, "DEBUG", format, args...)
	}
}

// SetupLogFile redirects os.Stdout and os.Stderr to the given file path, appending if it exists.
// The file is created with 0o600 permissions to protect potentially sensitive log content.
func SetupLogFile(path string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := safefile.OpenAppend(path)
	if err != nil {
		return err
	}
	os.Stdout = f
	os.Stderr = f
	return nil
}
