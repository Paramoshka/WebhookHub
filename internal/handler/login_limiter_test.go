package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginLimiterBlocksAfterFailures(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter()
	limiter.now = func() time.Time { return now }

	if !limiter.allow("203.0.113.7") {
		t.Fatal("expected first attempt to be allowed")
	}
	for i := 0; i < loginMaxAttempts; i++ {
		limiter.fail("203.0.113.7")
	}
	if limiter.allow("203.0.113.7") {
		t.Fatal("expected client to be blocked after max failed attempts")
	}
	if !limiter.allow("198.51.100.4") {
		t.Fatal("other clients must not be blocked")
	}

	now = now.Add(loginWindow + time.Second)
	if !limiter.allow("203.0.113.7") {
		t.Fatal("expected block to expire after the window")
	}
}

func TestLoginLimiterResetClearsFailures(t *testing.T) {
	limiter := newLoginLimiter()
	for i := 0; i < loginMaxAttempts; i++ {
		limiter.fail("203.0.113.7")
	}
	limiter.reset("203.0.113.7")
	if !limiter.allow("203.0.113.7") {
		t.Fatal("expected reset to clear failures")
	}
	limiter.fail("203.0.113.7")
	if !limiter.allow("203.0.113.7") {
		t.Fatal("expected counter to restart after reset")
	}
	if len(limiter.attempts) != 1 || limiter.attempts["203.0.113.7"].count != 1 {
		t.Fatalf("expected a single fresh attempt, got %+v", limiter.attempts)
	}
}

func TestLoginLimiterDropsExpiredEntries(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter()
	limiter.now = func() time.Time { return now }
	limiter.fail("203.0.113.7")

	now = now.Add(loginWindow + time.Second)
	limiter.allow("198.51.100.4")
	if len(limiter.attempts) != 0 {
		t.Fatalf("expected expired entries to be dropped, got %+v", limiter.attempts)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name      string
		remote    string
		forwarded string
		trust     bool
		want      string
	}{
		{name: "remote address", remote: "203.0.113.7:5555", want: "203.0.113.7"},
		{name: "remote address without port", remote: "203.0.113.7", want: "203.0.113.7"},
		{name: "forwarded ignored by default", remote: "203.0.113.7:5555", forwarded: "198.51.100.1, 10.0.0.1", want: "203.0.113.7"},
		{name: "forwarded first hop when trusted", remote: "10.0.0.1:5555", forwarded: "198.51.100.1, 10.0.0.1", trust: true, want: "198.51.100.1"},
		{name: "forwarded without trust falls back", remote: "10.0.0.1:5555", forwarded: "198.51.100.1", want: "10.0.0.1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/login", nil)
			request.RemoteAddr = test.remote
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-For", test.forwarded)
			}
			if got := clientIP(request, test.trust); got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestLoginReturnsTooManyRequestsWhenBlocked(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < loginMaxAttempts; i++ {
		auth.loginLimiter.fail("203.0.113.7")
	}

	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin%40example.com&password=secret"))
	request.RemoteAddr = "203.0.113.7:5555"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	auth.Login(nil)(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, response.Code)
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}
