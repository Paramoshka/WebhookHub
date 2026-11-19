package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type testReceiveStore struct {
	rule       model.ForwardingRule
	ruleErr    error
	saveErr    error
	ruleSource string
	saved      *model.Webhook
}

func (s *testReceiveStore) GetForwardingRule(_ context.Context, source string) (model.ForwardingRule, error) {
	s.ruleSource = source
	return s.rule, s.ruleErr
}

func (s *testReceiveStore) Save(_ context.Context, h *model.Webhook) error {
	if s.saveErr == nil {
		s.saved = h
	}
	return s.saveErr
}

func TestReceiveWebhookStoresEventWithoutRule(t *testing.T) {
	store := &testReceiveStore{ruleErr: storage.ErrNotFound}
	request := httptest.NewRequest(http.MethodPost, "/hook/stripe", strings.NewReader(`{"event":"demo.created"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	ReceiveWebhook(store, 1024)(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.Code)
	}
	if store.ruleSource != "stripe" {
		t.Fatalf("expected rule lookup for stripe, got %q", store.ruleSource)
	}
	if store.saved == nil {
		t.Fatal("webhook was not persisted")
	}
	if store.saved.Source != "stripe" || store.saved.Status != "pending" || string(store.saved.Payload) != `{"event":"demo.created"}` {
		t.Fatalf("unexpected stored webhook: %+v", store.saved)
	}
	if store.saved.ReceivedAt.IsZero() {
		t.Fatal("stored webhook has no receive timestamp")
	}
	if !strings.Contains(store.saved.Headers, `"Content-Type":["application/json"]`) {
		t.Fatalf("stored headers missing Content-Type: %s", store.saved.Headers)
	}
}

func TestReceiveWebhookVerifiesIncomingSignature(t *testing.T) {
	payload := `{"event":"demo.created"}`
	secret := "whsec_test_secret"
	for _, test := range []struct {
		name       string
		rule       model.ForwardingRule
		headerName string
		header     string
		body       string
		want       int
	}{
		{
			name:       "valid default header",
			rule:       model.ForwardingRule{VerifySecret: secret},
			headerName: hmacsig.DefaultIncomingHeader,
			header:     hmacsig.SignHeader(secret, []byte(payload), time.Now()),
			body:       payload,
			want:       http.StatusAccepted,
		},
		{
			name:       "valid custom header",
			rule:       model.ForwardingRule{VerifySecret: secret, VerifyHeader: "X-Custom-Signature"},
			headerName: "X-Custom-Signature",
			header:     hmacsig.SignHeader(secret, []byte(payload), time.Now()),
			body:       payload,
			want:       http.StatusAccepted,
		},
		{
			name:       "signature in wrong header is ignored",
			rule:       model.ForwardingRule{VerifySecret: secret, VerifyHeader: "X-Custom-Signature"},
			headerName: hmacsig.DefaultIncomingHeader,
			header:     hmacsig.SignHeader(secret, []byte(payload), time.Now()),
			body:       payload,
			want:       http.StatusUnauthorized,
		},
		{
			name:       "tampered payload",
			rule:       model.ForwardingRule{VerifySecret: secret},
			headerName: hmacsig.DefaultIncomingHeader,
			header:     hmacsig.SignHeader(secret, []byte(payload), time.Now()),
			body:       `{"event":"tampered"}`,
			want:       http.StatusUnauthorized,
		},
		{
			name:       "missing signature",
			rule:       model.ForwardingRule{VerifySecret: secret},
			headerName: hmacsig.DefaultIncomingHeader,
			body:       payload,
			want:       http.StatusUnauthorized,
		},
		{
			name:       "expired timestamp",
			rule:       model.ForwardingRule{VerifySecret: secret, ToleranceSeconds: 300},
			headerName: hmacsig.DefaultIncomingHeader,
			header:     hmacsig.SignHeader(secret, []byte(payload), time.Now().Add(-10*time.Minute)),
			body:       payload,
			want:       http.StatusUnauthorized,
		},
		{
			name:       "zero tolerance disables timestamp check",
			rule:       model.ForwardingRule{VerifySecret: secret},
			headerName: hmacsig.DefaultIncomingHeader,
			header:     hmacsig.SignHeader(secret, []byte(payload), time.Now().Add(-10*time.Minute)),
			body:       payload,
			want:       http.StatusAccepted,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testReceiveStore{rule: test.rule}
			request := httptest.NewRequest(http.MethodPost, "/hook/stripe", strings.NewReader(test.body))
			if test.header != "" {
				request.Header.Set(test.headerName, test.header)
			}
			response := httptest.NewRecorder()

			ReceiveWebhook(store, 1024)(response, request)

			if response.Code != test.want {
				t.Fatalf("expected status %d, got %d", test.want, response.Code)
			}
			if test.want == http.StatusAccepted && store.saved == nil {
				t.Fatal("accepted webhook was not persisted")
			}
			if test.want != http.StatusAccepted && store.saved != nil {
				t.Fatal("rejected webhook was persisted")
			}
		})
	}
}

func TestReceiveWebhookMapsStorageErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		ruleErr error
		saveErr error
		want    int
	}{
		{name: "rule lookup unavailable", ruleErr: errors.New("down"), want: http.StatusServiceUnavailable},
		{name: "persist unavailable", saveErr: errors.New("down"), want: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testReceiveStore{ruleErr: test.ruleErr, saveErr: test.saveErr}
			request := httptest.NewRequest(http.MethodPost, "/hook/stripe", strings.NewReader("{}"))
			response := httptest.NewRecorder()

			ReceiveWebhook(store, 1024)(response, request)

			if response.Code != test.want {
				t.Fatalf("expected status %d, got %d", test.want, response.Code)
			}
			if store.saved != nil {
				t.Fatal("webhook was persisted despite storage failure")
			}
		})
	}
}
