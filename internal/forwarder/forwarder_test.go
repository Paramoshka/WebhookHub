package forwarder

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"webhookhub/internal/model"
)

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
	status, body, errorMessage, success := performDeliveryAttemptWithClient(
		context.Background(),
		client,
		model.ForwardingRule{Target: "https://example.com/hook"},
		&model.Webhook{Payload: payload},
	)

	if !success || status != http.StatusOK || errorMessage != "" {
		t.Fatalf("unexpected result: success=%v status=%d error=%q", success, status, errorMessage)
	}
	if string(body) != "accepted" || receivedBody != string(payload) {
		t.Fatalf("unexpected bodies: response=%q request=%q", body, receivedBody)
	}
}

func TestPerformDeliveryAttemptHonorsCancellation(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, r.Context().Err()
	})}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, errorMessage, success := performDeliveryAttemptWithClient(
		ctx,
		client,
		model.ForwardingRule{Target: "https://example.com/hook"},
		&model.Webhook{Payload: []byte("{}")},
	)

	if success || !strings.Contains(errorMessage, context.Canceled.Error()) {
		t.Fatalf("expected cancelled delivery, got success=%v error=%q", success, errorMessage)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
