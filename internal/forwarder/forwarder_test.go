package forwarder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
)

func TestForwardPreservesContentTypeAndSignature(t *testing.T) {
	for _, contentType := range []string{"application/json; charset=utf-8", "application/xml", "application/x-www-form-urlencoded", "multipart/form-data; boundary=original", "application/octet-stream", ""} {
		t.Run(contentType, func(t *testing.T) {
			payload := []byte{0, 1, 0xff, '\r', '\n'}
			headers := http.Header{"Authorization": {"private"}, hmacsig.OutgoingHeader: {"old-signature"}}
			if contentType != "" {
				headers.Set("Content-Type", contentType)
			}
			encoded, err := json.Marshal(headers)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(body, payload) || r.Header.Get("Content-Type") != contentType {
					t.Fatal("payload or content type changed")
				}
				if r.Header.Get("Authorization") != "" {
					t.Fatal("authorization forwarded unexpectedly")
				}
				if err := hmacsig.VerifyHeader("secret", r.Header.Get(hmacsig.OutgoingHeader), body, time.Now(), time.Minute); err != nil {
					t.Fatalf("outgoing signature: %v", err)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
			})}
			_, message, success := performDeliveryAttemptWithClient(context.Background(), client, model.ForwardingRule{Target: "https://example.com", OutgoingSecret: "secret"}, &model.Webhook{Payload: payload, Headers: string(encoded)}, DefaultDeliveryTimeout)
			if !success {
				t.Fatal(message)
			}
		})
	}
}

func TestForwardRejectsCorruptStoredHeaders(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid stored headers must not be sent")
		return nil, nil
	})}
	response, message, success := performDeliveryAttemptWithClient(context.Background(), client, model.ForwardingRule{Target: "https://example.com"}, &model.Webhook{Headers: `{"Content-Type":`}, DefaultDeliveryTimeout)
	if success || response.Captured || !strings.Contains(message, "decode stored request headers") {
		t.Fatalf("unexpected result: %+v %q", response, message)
	}
}

func TestDeliveryResponseCapture(t *testing.T) {
	for _, size := range []int{0, int(maxBody) - 1, int(maxBody), int(maxBody) + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			body := bytes.Repeat([]byte{0xff}, size)
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 500, Header: http.Header{"X-Request-Id": {"first", "second"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
			})}
			response, message, success := performDeliveryAttemptWithClient(context.Background(), client, model.ForwardingRule{Target: "https://example.com"}, &model.Webhook{}, DefaultDeliveryTimeout)
			if success || message == "" || !response.Captured || response.HTTPStatus != 500 || response.Truncated != (size > int(maxBody)) {
				t.Fatalf("unexpected response metadata: status=%d captured=%v truncated=%v error=%q", response.HTTPStatus, response.Captured, response.Truncated, message)
			}
			if !bytes.Equal(response.Body, body[:min(size, int(maxBody))]) || !strings.Contains(response.Headers, `"X-Request-Id":["first","second"]`) {
				t.Fatal("response body or repeated headers were changed")
			}
		})
	}
}

func TestDeliveryResponseReadFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(io.MultiReader(strings.NewReader("partial"), failingReader{}))}, nil
	})}
	response, message, success := performDeliveryAttemptWithClient(context.Background(), client, model.ForwardingRule{Target: "https://example.com"}, &model.Webhook{}, DefaultDeliveryTimeout)
	if success || !response.Captured || string(response.Body) != "partial" || !strings.Contains(message, "broken stream") {
		t.Fatalf("partial response lost: %+v %q", response, message)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("broken stream") }

func TestRetryDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
		{attempt: 10, want: 30 * time.Second},
	}

	for _, test := range tests {
		if got := retryDelay(2, test.attempt); got != test.want {
			t.Errorf("attempt %d: expected %s, got %s", test.attempt, test.want, got)
		}
	}
}

func TestPerformDeliveryAttempt(t *testing.T) {
	var receivedBody string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		receivedBody = string(body)
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("expected application/json, got %q", contentType)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("accepted")),
			Header:     make(http.Header),
		}, nil
	})}

	payload := []byte(`{"event":"created"}`)
	response, errorMessage, success := performDeliveryAttemptWithClient(
		context.Background(),
		client,
		model.ForwardingRule{Target: "https://example.com/hook"},
		&model.Webhook{Payload: payload},
		DefaultDeliveryTimeout,
	)

	if !success || response.HTTPStatus != http.StatusOK || errorMessage != "" {
		t.Fatalf("unexpected result: success=%v status=%d error=%q", success, response.HTTPStatus, errorMessage)
	}
	if string(response.Body) != "accepted" || receivedBody != string(payload) {
		t.Fatalf("unexpected bodies: response=%q request=%q", response.Body, receivedBody)
	}
}

func TestPerformDeliveryAttemptHonorsCancellation(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, r.Context().Err()
	})}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, errorMessage, success := performDeliveryAttemptWithClient(
		ctx,
		client,
		model.ForwardingRule{Target: "https://example.com/hook"},
		&model.Webhook{Payload: []byte("{}")},
		DefaultDeliveryTimeout,
	)

	if success || !strings.Contains(errorMessage, context.Canceled.Error()) {
		t.Fatalf("expected cancelled delivery, got success=%v error=%q", success, errorMessage)
	}
}

func TestPerformDeliveryAttemptHonorsTimeout(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}

	_, errorMessage, success := performDeliveryAttemptWithClient(
		context.Background(),
		client,
		model.ForwardingRule{Target: "https://example.com/hook"},
		&model.Webhook{Payload: []byte("{}")},
		10*time.Millisecond,
	)

	if success || !strings.Contains(errorMessage, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected timed out delivery, got success=%v error=%q", success, errorMessage)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDeliveryDeadlineReservesLeaseTime(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		remaining, timeout, parent time.Duration
	}{
		{"lease bounds request", 6 * time.Second, 20 * time.Second, time.Minute},
		{"configured timeout", time.Minute, 2 * time.Second, time.Minute},
		{"parent deadline", time.Minute, 5 * time.Second, time.Second},
		{"zero timeout defaults", time.Minute, 0, time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lease := time.Now().Add(tt.remaining)
			parent, cancel := context.WithTimeout(context.Background(), tt.parent)
			defer cancel()
			start := time.Now()
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if !ok || deadline.After(lease.Add(-DeliveryFinalizationReserve)) {
					t.Fatal("request exceeds lease reserve")
				}
				limit := tt.timeout
				if limit <= 0 {
					limit = DefaultDeliveryTimeout
				}
				expected := min(limit, tt.remaining-DeliveryFinalizationReserve, tt.parent)
				if delta := deadline.Sub(start); delta > expected+100*time.Millisecond || delta < expected-100*time.Millisecond {
					t.Fatalf("unexpected deadline offset %s, want about %s", delta, expected)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
			})}
			_, message, success := performDeliveryAttemptWithClient(parent, client, model.ForwardingRule{Target: "https://example.com"}, &model.Webhook{DeliveryLeaseUntil: &lease}, tt.timeout)
			if !success {
				t.Fatal(message)
			}
		})
	}
}

func TestDeliveryDoesNotSendWithoutLeaseBudget(t *testing.T) {
	for _, remaining := range []time.Duration{-time.Second, DeliveryFinalizationReserve - time.Millisecond} {
		lease := time.Now().Add(remaining)
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("request sent after lease budget exhausted")
			return nil, nil
		})}
		_, message, success := performDeliveryAttemptWithClient(context.Background(), client, model.ForwardingRule{Target: "https://example.com"}, &model.Webhook{DeliveryLeaseUntil: &lease}, DefaultDeliveryTimeout)
		if success || !strings.Contains(message, context.DeadlineExceeded.Error()) {
			t.Fatalf("expected deadline error, got %v %s", success, message)
		}
	}
}
