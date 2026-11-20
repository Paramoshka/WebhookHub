package forwarder

import (
	"context"
	"testing"
	"time"

	"webhookhub/internal/model"
)

func TestPlanDeliveryResultSchedulesRetry(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	rule := model.ForwardingRule{Target: "https://example.com/hook"}
	outcome := deliveryOutcome{
		response:     model.DeliveryResponse{HTTPStatus: 500, Captured: true},
		errorMessage: "target responded with status 500",
	}

	result := planDeliveryResult(rule, 0, outcome, nil, now)

	if result.Status != "failed" {
		t.Fatalf("expected failed, got %q", result.Status)
	}
	if result.NextRetryAt == nil {
		t.Fatal("expected a retry to be scheduled")
	}
	if want := now.Add(2 * time.Second); !result.NextRetryAt.Equal(want) {
		t.Fatalf("expected retry at %s, got %s", want, result.NextRetryAt)
	}
	if result.ErrorMessage != "target responded with status 500" || result.Response.HTTPStatus != 500 {
		t.Fatalf("attempt outcome lost: %+v", result)
	}
}

func TestPlanDeliveryResultTransitions(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	failed := deliveryOutcome{
		response:     model.DeliveryResponse{HTTPStatus: 500, Body: []byte("boom"), Captured: true},
		errorMessage: "target responded with status 500",
	}
	succeeded := deliveryOutcome{response: model.DeliveryResponse{HTTPStatus: 200, Captured: true}, success: true}
	retryIn := func(d time.Duration) *time.Duration { return &d }
	for _, test := range []struct {
		name         string
		rule         model.ForwardingRule
		failureCount int
		outcome      deliveryOutcome
		cancelErr    error
		wantStatus   string
		wantRetryIn  *time.Duration
		wantError    string
	}{
		{name: "success", rule: model.ForwardingRule{Target: "https://example.com"}, outcome: succeeded, wantStatus: "success"},
		{name: "failure schedules exponential backoff", rule: model.ForwardingRule{Target: "https://example.com", RetryMaxAttempts: 5, RetryBackoffSeconds: 2}, failureCount: 3, outcome: failed, wantStatus: "failed", wantRetryIn: retryIn(16 * time.Second), wantError: "target responded with status 500"},
		{name: "exhausted attempts leave no retry", rule: model.ForwardingRule{Target: "https://example.com", RetryMaxAttempts: 3}, failureCount: 2, outcome: failed, wantStatus: "failed", wantError: "target responded with status 500"},
		{name: "default policy exhausts after three attempts", rule: model.ForwardingRule{Target: "https://example.com"}, failureCount: 2, outcome: failed, wantStatus: "failed", wantError: "target responded with status 500"},
		{name: "single attempt policy exhausts immediately", rule: model.ForwardingRule{Target: "https://example.com", RetryMaxAttempts: 1}, outcome: failed, wantStatus: "failed", wantError: "target responded with status 500"},
		{name: "backoff is capped", rule: model.ForwardingRule{Target: "https://example.com", RetryBackoffSeconds: 60}, outcome: failed, wantStatus: "failed", wantRetryIn: retryIn(30 * time.Second), wantError: "target responded with status 500"},
		{name: "no target is skipped", rule: model.ForwardingRule{}, outcome: deliveryOutcome{}, wantStatus: "skipped", wantError: "no forwarding target configured"},
		{name: "cancel overrides retry", rule: model.ForwardingRule{Target: "https://example.com"}, outcome: failed, cancelErr: context.Canceled, wantStatus: "cancelled", wantError: "context canceled"},
		{name: "cancel overrides success", rule: model.ForwardingRule{Target: "https://example.com"}, outcome: succeeded, cancelErr: context.Canceled, wantStatus: "cancelled", wantError: "context canceled"},
		{name: "cancel overrides skip", rule: model.ForwardingRule{}, cancelErr: context.DeadlineExceeded, wantStatus: "cancelled", wantError: "context deadline exceeded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := planDeliveryResult(test.rule, test.failureCount, test.outcome, test.cancelErr, now)
			if result.Status != test.wantStatus {
				t.Fatalf("status=%q, want %q", result.Status, test.wantStatus)
			}
			if result.ErrorMessage != test.wantError {
				t.Fatalf("error=%q, want %q", result.ErrorMessage, test.wantError)
			}
			switch {
			case test.wantRetryIn == nil && result.NextRetryAt != nil:
				t.Fatalf("unexpected retry at %s", result.NextRetryAt)
			case test.wantRetryIn != nil:
				if result.NextRetryAt == nil {
					t.Fatal("expected a retry")
				}
				if want := now.Add(*test.wantRetryIn); !result.NextRetryAt.Equal(want) {
					t.Fatalf("retry at %s, want %s", result.NextRetryAt, want)
				}
			}
			if result.Response.HTTPStatus != test.outcome.response.HTTPStatus {
				t.Fatalf("response lost: %+v", result.Response)
			}
		})
	}
}
