package handler

import (
	"bytes"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type testInspectStore struct {
	webhook     model.Webhook
	findErr     error
	attemptsErr error
}

type testAttemptStore struct {
	attempt              model.DeliveryAttempt
	err                  error
	webhookID, attemptID uint
}

func (s *testAttemptStore) DeliveryAttemptByID(webhookID, attemptID uint) (model.DeliveryAttempt, error) {
	s.webhookID, s.attemptID = webhookID, attemptID
	return s.attempt, s.err
}

func TestInspectDeliveryAttempt(t *testing.T) {
	t.Chdir(filepath.Join(projectTemplateDir(t), "..", ".."))
	for _, test := range []struct {
		name     string
		id       string
		err      error
		captured bool
		want     int
	}{
		{"response", "7", nil, true, 200},
		{"legacy", "7", nil, false, 200},
		{"invalid", "0", nil, false, 400},
		{"not found", "7", storage.ErrNotFound, false, 404},
		{"unavailable", "7", errors.New("down"), false, 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testAttemptStore{err: test.err, attempt: model.DeliveryAttempt{ID: 7, WebhookID: 42, ResponseCaptured: test.captured, ResponseTruncated: true,
				ResponseBody: []byte("<script>bad()</script>"), ResponseHeaders: `{"X-Test":["<img src=x>","second"]}`}}
			r := httptest.NewRequest("GET", "/webhooks/42/attempts/7", nil)
			r.SetPathValue("id", "42")
			r.SetPathValue("attemptID", test.id)
			w := httptest.NewRecorder()
			InspectDeliveryAttempt(store)(w, r)
			if w.Code != test.want {
				t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
			}
			if test.want == 200 {
				if store.webhookID != 42 || store.attemptID != 7 {
					t.Fatal("lookup must be scoped to webhook")
				}
				body := w.Body.String()
				if strings.Contains(body, "<script>bad") || strings.Contains(body, "<img src=x>") {
					t.Fatal("unsafe response rendering")
				}
				if test.captured && (!strings.Contains(body, "Response truncated") || !strings.Contains(body, "second")) {
					t.Fatal("missing response details")
				}
				if !test.captured && !strings.Contains(body, "No response was saved") {
					t.Fatal("missing legacy message")
				}
			}
		})
	}
}

func (s testInspectStore) FindByID(int) (model.Webhook, error) {
	return s.webhook, s.findErr
}

func (s testInspectStore) DeliveryAttemptsByWebhook(uint) ([]model.DeliveryAttempt, error) {
	return nil, s.attemptsErr
}

func TestInspectBody(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   []byte
		json   bool
		binary bool
	}{
		{name: "json", body: []byte(`{"id":900719925474099312345,"name":"Привет"}`), json: true},
		{name: "plain text", body: []byte("line one\r\nline two\tПривет")},
		{name: "invalid json", body: []byte(`{"incomplete":`)},
		{name: "empty"},
		{name: "invalid utf8", body: []byte{0xff, 0xfe}, binary: true},
		{name: "zero byte", body: []byte("before\x00after"), binary: true},
		{name: "control bytes", body: []byte{1, 2, 3}, binary: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := inspectBody("payload", "Payload", test.body)
			if view.JSON != test.json || view.Binary != test.binary || view.Size != len(test.body) {
				t.Fatalf("unexpected body view: %+v", view)
			}
			if !test.binary && view.Raw != string(test.body) {
				t.Fatal("raw text was changed")
			}
			if test.json && (!strings.Contains(view.Formatted, "900719925474099312345") || !strings.Contains(view.Formatted, "\n  ")) {
				t.Fatalf("JSON formatting changed number precision: %s", view.Formatted)
			}
		})
	}
	body := bytes.Repeat([]byte{0xff}, 300)
	view := inspectBody("payload", "Payload", body)
	if view.Size != 300 || view.Raw != hex.Dump(body[:256]) {
		t.Fatal("binary preview must be limited to the first 256 bytes")
	}
}

func TestDownloadWebhookPayloadPreservesBytes(t *testing.T) {
	for _, body := range [][]byte{nil, []byte("\xef\xbb\xbftext\r\n"), {0, 1, 0xff, 0xfe}} {
		store := testInspectStore{
			webhook:     model.Webhook{ID: 42, Payload: body},
			attemptsErr: errors.New("download must not load delivery history"),
		}
		request := httptest.NewRequest(http.MethodGet, "/webhooks/42/payload", nil)
		request.SetPathValue("id", "42")
		response := httptest.NewRecorder()
		DownloadWebhookPayload(store)(response, request)
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), body) {
			t.Fatalf("download changed bytes: code=%d body=%x", response.Code, response.Body.Bytes())
		}
		if response.Header().Get("Content-Type") != "application/octet-stream" ||
			response.Header().Get("Content-Disposition") != `attachment; filename="webhook-42-payload.bin"` {
			t.Fatalf("unexpected download headers: %v", response.Header())
		}
	}
}

func TestInspectEndpointsReportErrors(t *testing.T) {
	for _, endpoint := range []struct {
		name string
		new  func(webhookInspectStore) http.HandlerFunc
	}{
		{name: "page", new: InspectWebhook},
		{name: "delivery", new: InspectDeliveryPartial},
		{name: "download", new: DownloadWebhookPayload},
	} {
		for _, test := range []struct {
			name string
			id   string
			err  error
			want int
		}{
			{name: "invalid", id: "1 OR 1=1", want: http.StatusBadRequest},
			{name: "zero", id: "0", want: http.StatusBadRequest},
			{name: "overflow", id: "999999999999999999999999", want: http.StatusBadRequest},
			{name: "missing", id: "42", err: storage.ErrNotFound, want: http.StatusNotFound},
			{name: "database unavailable", id: "42", err: errors.New("down"), want: http.StatusServiceUnavailable},
		} {
			t.Run(endpoint.name+"/"+test.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.SetPathValue("id", test.id)
				response := httptest.NewRecorder()
				endpoint.new(testInspectStore{findErr: test.err})(response, request)
				if response.Code != test.want {
					t.Fatalf("expected %d, got %d", test.want, response.Code)
				}
			})
		}
	}
}

func TestInspectPageAndDeliveryRenderSafely(t *testing.T) {
	t.Chdir(filepath.Join(projectTemplateDir(t), "..", ".."))
	store := testInspectStore{webhook: model.Webhook{
		ID: 42, Source: "inspect", Status: "processing", ReceivedAt: time.Now(),
		Headers:  `{"X-Repeat":["first","second"],"X-Html":["<img src=x onerror=alert(1)>"]}`,
		Payload:  []byte(`{"value":"<script>alert(1)</script>","id":900719925474099312345}`),
		Response: []byte("<script>response()</script>"),
	}}
	for _, handler := range []http.HandlerFunc{InspectWebhook(store), InspectDeliveryPartial(store)} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("id", "42")
		response := httptest.NewRecorder()
		handler(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("render failed: %d %s", response.Code, response.Body.String())
		}
		body := response.Body.String()
		if strings.Contains(body, "<script>alert") || strings.Contains(body, "<script>response") || strings.Contains(body, "<img src=x") {
			t.Fatal("untrusted content was rendered as HTML")
		}
		if !strings.Contains(body, "disabled>Replay") || !strings.Contains(body, "&lt;script&gt;response") {
			t.Fatal("missing processing state or escaped response")
		}
		if strings.Contains(body, "<!DOCTYPE html>") {
			for _, want := range []string{"900719925474099312345", "first", "second", "/static/style.css", "/static/inspect.js", "UTC"} {
				if !strings.Contains(body, want) {
					t.Fatalf("page is missing %q", want)
				}
			}
		} else if strings.Contains(body, "payload-title") || strings.Contains(body, "Request headers") {
			t.Fatal("polling response must not replace payload or headers")
		}
	}
}

func TestRequireAuthRedirectsHTMXToLogin(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false)
	if err != nil {
		t.Fatal(err)
	}
	next := auth.RequireAuth(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthenticated handler called") })
	for _, hx := range []bool{false, true} {
		for _, cookie := range []string{"", "invalid-session"} {
			request := httptest.NewRequest(http.MethodGet, "/partials/webhook/42/delivery", nil)
			if hx {
				request.Header.Set("HX-Request", "true")
			}
			if cookie != "" {
				request.AddCookie(&http.Cookie{Name: "session", Value: cookie})
			}
			response := httptest.NewRecorder()
			next(response, request)
			if hx {
				if response.Code != http.StatusUnauthorized || response.Header().Get("HX-Redirect") != "/login" {
					t.Fatalf("expected HTMX navigation to login, got %d %v", response.Code, response.Header())
				}
			} else if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
				t.Fatalf("expected ordinary login redirect, got %d %v", response.Code, response.Header())
			}
		}
	}
}
