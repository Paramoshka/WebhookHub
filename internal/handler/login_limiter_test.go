package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func reserveLogin(t *testing.T, l *loginLimiter, ip string) *loginReservation {
	t.Helper()
	r, retry := l.reserve(ip)
	if r == nil {
		t.Fatalf("reservation blocked for %s, retry %d", ip, retry)
	}
	return r
}

func TestLoginLimiterFailuresAndRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	l := newLoginLimiter()
	l.now = func() time.Time { return now }
	for i := 0; i < loginMaxAttempts; i++ {
		reserveLogin(t, l, "client").finish(loginFailed)
	}
	if r, retry := l.reserve("client"); r != nil || retry != 900 {
		t.Fatalf("expected 900s block, got %v %d", r, retry)
	}
	reserveLogin(t, l, "other").finish(loginAborted)
	now = now.Add(loginWindow - time.Second/2)
	if r, retry := l.reserve("client"); r != nil || retry != 1 {
		t.Fatalf("expected rounded 1s block, got %v %d", r, retry)
	}
	now = now.Add(time.Second / 2)
	reserveLogin(t, l, "client").finish(loginFailed)
}

func TestLoginLimiterConcurrentReservations(t *testing.T) {
	l := newLoginLimiter()
	var ready, done sync.WaitGroup
	var admitted atomic.Int32
	release := make(chan struct{})
	ready.Add(100)
	done.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer done.Done()
			r, _ := l.reserve("client")
			if r != nil {
				admitted.Add(1)
			}
			ready.Done()
			<-release
			if r != nil {
				r.finish(loginFailed)
			}
		}()
	}
	ready.Wait()
	close(release)
	done.Wait()
	if got := admitted.Load(); got != loginMaxAttempts {
		t.Fatalf("admitted %d concurrent checks, want %d", got, loginMaxAttempts)
	}
	if r, _ := l.reserve("client"); r != nil {
		t.Fatal("failures must remain blocked")
	}
}

func TestLoginLimiterSuccessKeepsPendingChecks(t *testing.T) {
	l := newLoginLimiter()
	reserveLogin(t, l, "client").finish(loginFailed)
	pending := make([]*loginReservation, 0, 9)
	for i := 0; i < 9; i++ {
		pending = append(pending, reserveLogin(t, l, "client"))
	}
	pending[0].finish(loginSucceeded)
	for i := 0; i < 2; i++ {
		pending = append(pending, reserveLogin(t, l, "client"))
	}
	if r, retry := l.reserve("client"); r != nil || retry != 1 {
		t.Fatal("success dropped pending reservations")
	}
	for _, r := range pending[1:] {
		r.finish(loginAborted)
	}
	if l.attempts["client"].failures != 0 || l.attempts["client"].pending != 0 {
		t.Fatal("aborted checks consumed failures or leaked slots")
	}
	for i := 0; i < 20; i++ {
		reserveLogin(t, l, "client").finish(loginAborted)
	}
}

func TestLoginLimiterOldCompletionAndSweep(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	l := newLoginLimiter()
	l.now = func() time.Time { return now }
	oldFailure := reserveLogin(t, l, "client")
	oldSuccess := reserveLogin(t, l, "client")
	reserveLogin(t, l, "idle").finish(loginFailed)
	now = now.Add(loginWindow)
	reserveLogin(t, l, "client").finish(loginFailed)
	if _, exists := l.attempts["idle"]; exists {
		t.Fatal("expired idle entry retained")
	}
	oldFailure.finish(loginFailed)
	oldSuccess.finish(loginSucceeded)
	entry := l.attempts["client"]
	if entry.failures != 1 || entry.pending != 0 {
		t.Fatalf("old completions changed new window: %+v", entry)
	}
	now = now.Add(loginWindow)
	reserveLogin(t, l, "other").finish(loginAborted)
	if _, exists := l.attempts["client"]; exists {
		t.Fatal("completed expired entry retained")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, remote string
		forwarded    []string
		trust        bool
		want         string
	}{
		{"remote", "203.0.113.7:5555", nil, false, "203.0.113.7"},
		{"no port", "203.0.113.7", nil, false, "203.0.113.7"},
		{"ignored", "203.0.113.7:5555", []string{"6.6.6.6, 10.0.0.1"}, false, "203.0.113.7"},
		{"appended", "10.0.0.1:5555", []string{"6.6.6.6, 203.0.113.7"}, true, "203.0.113.7"},
		{"overwritten", "10.0.0.1:5555", []string{"203.0.113.7"}, true, "203.0.113.7"},
		{"multiple lines", "10.0.0.1:5555", []string{"6.6.6.6", "203.0.113.7"}, true, "203.0.113.7"},
		{"invalid last", "10.0.0.1:5555", []string{"6.6.6.6, invalid"}, true, "10.0.0.1"},
		{"empty last", "10.0.0.1:5555", []string{"6.6.6.6,"}, true, "10.0.0.1"},
		{"empty line", "10.0.0.1:5555", []string{"6.6.6.6", ""}, true, "10.0.0.1"},
		{"IPv6", "[2001:db8::1]:5555", nil, false, "2001:db8::1"},
		{"canonical IPv6", "10.0.0.1:5555", []string{" 2001:0db8:0:0::1 "}, true, "2001:db8::1"},
		{"mapped IPv4", "10.0.0.1:5555", []string{"::ffff:203.0.113.7"}, true, "203.0.113.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/login", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.forwarded {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIP(r, tt.trust); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
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
		reserveLogin(t, auth.loginLimiter, "203.0.113.7").finish(loginFailed)
	}
	token, err := auth.loginCookies.Encode("login_csrf", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin%40example.com&password=secret&csrf_token=test-token"))
	request.RemoteAddr = "203.0.113.7:5555"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: "login_csrf", Value: token})
	response := httptest.NewRecorder()
	auth.Login(nil)(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "900" {
		t.Fatalf("unexpected blocked response: %d %v", response.Code, response.Header())
	}
}
