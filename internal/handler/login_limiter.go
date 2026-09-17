package handler

import (
	"net"
	"net/http"
	"net/netip"
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
	attempts  map[string]*loginAttempt
	nextSweep time.Time
	now       func() time.Time
}

type loginAttempt struct {
	failures  int
	pending   int
	windowEnd time.Time
}

type loginOutcome int

const (
	loginAborted loginOutcome = iota
	loginFailed
	loginSucceeded
)

type loginReservation struct {
	limiter   *loginLimiter
	entry     *loginAttempt
	windowEnd time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginAttempt), now: time.Now}
}

func (l *loginLimiter) reserve(key string) (*loginReservation, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)
	entry := l.attempts[key]
	if entry == nil {
		entry = &loginAttempt{}
		l.attempts[key] = entry
	}
	if !now.Before(entry.windowEnd) {
		entry.failures = 0
		entry.windowEnd = now.Add(loginWindow)
	}
	if entry.failures >= loginMaxAttempts {
		remaining := entry.windowEnd.Sub(now)
		return nil, int((remaining + time.Second - 1) / time.Second)
	}
	if entry.failures+entry.pending >= loginMaxAttempts {
		return nil, 1
	}
	entry.pending++
	return &loginReservation{limiter: l, entry: entry, windowEnd: entry.windowEnd}, 0
}

// finish is called once for each reservation, including database-error exits.
func (r *loginReservation) finish(outcome loginOutcome) {
	l := r.limiter
	l.mu.Lock()
	defer l.mu.Unlock()

	r.entry.pending--
	// An old request must not change the failures of a newer window.
	if !l.now().Before(r.windowEnd) || !r.entry.windowEnd.Equal(r.windowEnd) {
		return
	}
	switch outcome {
	case loginFailed:
		r.entry.failures++
	case loginSucceeded:
		r.entry.failures = 0
	}
}

// sweepLocked removes expired idle entries. It does not bound the number of
// client addresses seen within a window, and never drops active reservations.
func (l *loginLimiter) sweepLocked(now time.Time) {
	if now.Before(l.nextSweep) {
		return
	}
	for key, entry := range l.attempts {
		if entry.pending == 0 && !now.Before(entry.windowEnd) {
			delete(l.attempts, key)
		}
	}
	l.nextSweep = now.Add(loginWindow)
}

// Trusted deployments must have one proxy that appends or overwrites XFF and
// must prevent clients from reaching this server directly.
func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		values := r.Header.Values("X-Forwarded-For")
		if len(values) > 0 {
			last := values[len(values)-1]
			last = last[strings.LastIndex(last, ",")+1:]
			if ip, err := netip.ParseAddr(strings.TrimSpace(last)); err == nil {
				return ip.Unmap().String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap().String()
	}
	return host
}
