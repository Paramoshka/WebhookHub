package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"webhookhub/internal/storage"
)

type testPinger struct {
	err error
}

type testWebhookMutationStore struct {
	resetErr  error
	deleteErr error
	resetID   int
	deleteID  int
}

func (s *testWebhookMutationStore) ResetWebhookDeliveryState(id int) error {
	s.resetID = id
	return s.resetErr
}

func (s *testWebhookMutationStore) DeleteWebhook(id int) error {
	s.deleteID = id
	return s.deleteErr
}

func (p testPinger) Ping(context.Context) error {
	return p.err
}

func TestReceiveWebhookRejectsInvalidSource(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/hook/invalid/source", strings.NewReader("{}"))
	response := httptest.NewRecorder()

	ReceiveWebhook(nil, 1024)(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestReceiveWebhookRejectsLargePayload(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/hook/stripe", strings.NewReader("12345"))
	response := httptest.NewRecorder()

	ReceiveWebhook(nil, 4)(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, response.Code)
	}
}

func TestParseForwardingRuleFormValidatesSourceAndTarget(t *testing.T) {
	tests := []struct {
		name   string
		source string
		target string
	}{
		{name: "invalid source", source: "bad/source", target: "https://example.com/hook"},
		{name: "relative target", source: "stripe", target: "/hook"},
		{name: "target credentials", source: "stripe", target: "https://user:pass@example.com/hook"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := url.Values{"source": {test.source}, "target": {test.target}}
			request := httptest.NewRequest(http.MethodPost, "/forwarding/save", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}

			if _, err := parseForwardingRuleForm(request, nil); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestRequireCSRF(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false)
	if err != nil {
		t.Fatal(err)
	}

	next := auth.RequireCSRF(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("rejects invalid token", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/action", nil)
		request = request.WithContext(context.WithValue(request.Context(), csrfContextKey, "expected"))
		request.Header.Set("X-CSRF-Token", "wrong")
		response := httptest.NewRecorder()

		next(response, request)

		if response.Code != http.StatusForbidden {
			t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
		}
	})

	t.Run("accepts header token", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/action", nil)
		request = request.WithContext(context.WithValue(request.Context(), csrfContextKey, "expected"))
		request.Header.Set("X-CSRF-Token", "expected")
		response := httptest.NewRecorder()

		next(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
		}
	})
}

func TestNewAuthRejectsShortSessionKey(t *testing.T) {
	if _, err := NewAuth("too-short", false); err == nil {
		t.Fatal("expected short session key to be rejected")
	}
}

func TestLoginRequiresCSRFToken(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin%40example.com&password=password"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	auth.Login(nil)(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, response.Code)
	}
}

func TestReadyReflectsDatabaseState(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "ready", want: http.StatusOK},
		{name: "database unavailable", err: errors.New("down"), want: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			Ready(testPinger{err: test.err})(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if response.Code != test.want {
				t.Fatalf("expected status %d, got %d", test.want, response.Code)
			}
		})
	}
}

func TestReplayWebhookReturnsMutationStatus(t *testing.T) {
	tests := []struct {
		name string
		path string
		err  error
		want int
	}{
		{name: "invalid ID", path: "/api/webhooks/replay?id=invalid", want: http.StatusBadRequest},
		{name: "not found", path: "/api/webhooks/replay?id=42", err: storage.ErrNotFound, want: http.StatusNotFound},
		{name: "processing", path: "/api/webhooks/replay?id=42", err: storage.ErrWebhookProcessing, want: http.StatusConflict},
		{name: "database unavailable", path: "/api/webhooks/replay?id=42", err: errors.New("down"), want: http.StatusServiceUnavailable},
		{name: "requeued", path: "/api/webhooks/replay?id=42", want: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &testWebhookMutationStore{resetErr: test.err}
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			request.Header.Set("HX-Request", "true")
			response := httptest.NewRecorder()

			ReplayWebhook(store)(response, request)

			if response.Code != test.want {
				t.Fatalf("expected status %d, got %d", test.want, response.Code)
			}
			if test.path == "/api/webhooks/replay?id=42" && store.resetID != 42 {
				t.Fatalf("expected webhook ID 42, got %d", store.resetID)
			}
		})
	}
}

func TestDeleteWebhookReturnsMutationStatus(t *testing.T) {
	tests := []struct {
		name string
		path string
		err  error
		want int
	}{
		{name: "invalid ID", path: "/api/webhooks/delete?id=0", want: http.StatusBadRequest},
		{name: "not found", path: "/api/webhooks/delete?id=42", err: storage.ErrNotFound, want: http.StatusNotFound},
		{name: "processing", path: "/api/webhooks/delete?id=42", err: storage.ErrWebhookProcessing, want: http.StatusConflict},
		{name: "database unavailable", path: "/api/webhooks/delete?id=42", err: errors.New("down"), want: http.StatusServiceUnavailable},
		{name: "deleted", path: "/api/webhooks/delete?id=42", want: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &testWebhookMutationStore{deleteErr: test.err}
			request := httptest.NewRequest(http.MethodPost, test.path, nil)
			request.Header.Set("HX-Request", "true")
			response := httptest.NewRecorder()

			DeleteWebhook(store)(response, request)

			if response.Code != test.want {
				t.Fatalf("expected status %d, got %d", test.want, response.Code)
			}
			if test.path == "/api/webhooks/delete?id=42" && store.deleteID != 42 {
				t.Fatalf("expected webhook ID 42, got %d", store.deleteID)
			}
		})
	}
}
