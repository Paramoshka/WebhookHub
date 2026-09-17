package handler

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginMaxAttempts = 10
	loginWindow      = 15 * time.Minute
)

type loginLimiter struct {
	mu        sync.Mutex
	attempts  map[string]loginAttempt
	nextSweep time.Time
	now       func() time.Time
}

type loginAttempt struct {
	count     int
	windowEnd time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginAttempt), now: time.Now}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)

	entry, found := l.attempts[key]
	if !found || now.After(entry.windowEnd) {
		return true
	}
	return entry.count < loginMaxAttempts
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	entry := l.attempts[key]
	if now.After(entry.windowEnd) {
		entry = loginAttempt{windowEnd: now.Add(loginWindow)}
	}
	entry.count++
	l.attempts[key] = entry
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, key)
}

// sweepLocked drops entries whose window has passed. It runs at most once per
// window so the map cannot grow without bound under a distributed attack.
func (l *loginLimiter) sweepLocked(now time.Time) {
	if now.Before(l.nextSweep) {
		return
	}
	for key, entry := range l.attempts {
		if now.After(entry.windowEnd) {
			delete(l.attempts, key)
		}
	}
	l.nextSweep = now.Add(loginWindow)
}

// clientIP returns the request origin used to scope login rate limits. Forwarded
// headers are only honored when the deployment explicitly trusts its proxies,
// because clients can set X-Forwarded-For themselves.
func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			first, _, _ := strings.Cut(forwarded, ",")
			if value := strings.TrimSpace(first); value != "" {
				return value
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
