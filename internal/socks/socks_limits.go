package socks

import (
	"sync"
	"time"
)

const (
	defaultSocksMaxConns       = 256
	defaultSocksUDPIdleTimeout = 2 * time.Minute
	socksAuthFailureDelay      = 500 * time.Millisecond
	socksWarnInterval          = time.Minute
	maxLogRateLimiterKeys      = 4096
)

// logRateLimiter allows one log line per key per interval, so a client hammering the
// proxy cannot flood the log.
type logRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     map[string]time.Time
}

func newLogRateLimiter(interval time.Duration) *logRateLimiter {
	return &logRateLimiter{interval: interval, last: make(map[string]time.Time)}
}

func (l *logRateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t, ok := l.last[key]; ok && now.Sub(t) < l.interval {
		return false
	}
	if len(l.last) >= maxLogRateLimiterKeys {
		for k, t := range l.last {
			if now.Sub(t) >= l.interval {
				delete(l.last, k)
			}
		}
		if len(l.last) >= maxLogRateLimiterKeys {
			return false
		}
	}
	l.last[key] = now
	return true
}

func (l *logRateLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.last)
}
