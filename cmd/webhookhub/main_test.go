package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webhookhub/internal/handler"
)

func TestLoadConfigRejectsWeakSecrets(t *testing.T) {
	setValidConfigEnvironment(t)
	t.Setenv("SESSION_KEY", "short")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected weak session key to be rejected")
	}
}

func TestLoadConfigAcceptsValidEnvironment(t *testing.T) {
	setValidConfigEnvironment(t)

	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.DeliveryWorkers != 4 || config.MaxBodyBytes != 1<<20 {
		t.Fatalf("unexpected defaults: %+v", config)
	}
}

func TestRoutesEnforceWebhookMethod(t *testing.T) {
	auth, err := handler.NewAuth(strings.Repeat("a", 32), false, false)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hook/stripe", nil)
	response := httptest.NewRecorder()

	routes(nil, auth, 1024).ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}
}

func TestRoutesProtectInspect(t *testing.T) {
	auth, err := handler.NewAuth(strings.Repeat("a", 32), false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/webhooks/42", "/webhooks/42/payload", "/webhooks/42/attempts/7", "/partials/webhook/42", "/partials/webhook/42/delivery"} {
		response := httptest.NewRecorder()
		routes(nil, auth, 1024).ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
			t.Fatalf("%s must require login, got %d", path, response.Code)
		}
	}
}

func TestCheckReadiness(t *testing.T) {
	tests := []struct {
		name   string
		status int
		wantOK bool
	}{
		{name: "ready", status: http.StatusOK, wantOK: true},
		{name: "not ready", status: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: mainRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			})}

			err := checkReadiness(client, "http://127.0.0.1:8080/readyz")
			if test.wantOK && err != nil {
				t.Fatalf("expected readiness success, got %v", err)
			}
			if !test.wantOK && err == nil {
				t.Fatal("expected readiness failure")
			}
		})
	}
}

func TestCheckReadinessRejectsUnavailableEndpoint(t *testing.T) {
	client := &http.Client{Transport: mainRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}

	if err := checkReadiness(client, "http://127.0.0.1:8080/readyz"); err == nil {
		t.Fatal("expected unavailable endpoint to fail")
	}
}

func TestRunHealthcheckRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"invalid", "0", "65536"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("PORT", port)

			if err := runHealthcheck(); err == nil {
				t.Fatalf("expected PORT=%q to fail", port)
			}
		})
	}
}

type mainRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mainRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func setValidConfigEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("POSTGRES_HOST", "localhost")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_USER", "webhookhub")
	t.Setenv("POSTGRES_PASSWORD", "database-password")
	t.Setenv("POSTGRES_DB", "webhookhub")
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	t.Setenv("ADMIN_PASSWORD", "strong-password")
	t.Setenv("SESSION_KEY", strings.Repeat("s", 32))
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("PORT", "")
	t.Setenv("MAX_BODY_BYTES", "")
	t.Setenv("DELIVERY_WORKERS", "")
	t.Setenv("DELIVERY_POLL_INTERVAL", "")
	t.Setenv("DELIVERY_LEASE_DURATION", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	t.Setenv("RETENTION_ENABLED", "")
	t.Setenv("RETENTION_DAYS", "")
	t.Setenv("RETENTION_INTERVAL", "")
	t.Setenv("RETENTION_BATCH_SIZE", "")
}
