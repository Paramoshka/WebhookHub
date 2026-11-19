package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webhookhub/internal/storage"
)

type testMetricsStore struct {
	metrics storage.DeliveryMetrics
	err     error
}

func (s *testMetricsStore) DeliveryMetrics(context.Context) (storage.DeliveryMetrics, error) {
	return s.metrics, s.err
}

func TestDeliveryMetricsPartialRendersCounts(t *testing.T) {
	store := &testMetricsStore{metrics: storage.DeliveryMetrics{TotalWebhooks: 123, DeadLetterCount: 4, SuccessRate: 92.5}}
	request := httptest.NewRequest(http.MethodGet, "/partials/metrics", nil)
	response := httptest.NewRecorder()

	DeliveryMetricsPartial(store)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`<div class="metric-value">123</div>`, `<div class="metric-value">4</div>`, "92.5%"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in metrics fragment", want)
		}
	}
}

func TestDeliveryMetricsPartialMapsStorageErrors(t *testing.T) {
	store := &testMetricsStore{err: errors.New("down")}
	request := httptest.NewRequest(http.MethodGet, "/partials/metrics", nil)
	response := httptest.NewRecorder()

	DeliveryMetricsPartial(store)(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}
